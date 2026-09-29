/// <reference lib="webworker" />
// The calculation engine of the server, compiled to WebAssembly, running off the main thread. It answers
// preliminary numbers: a saved run is always computed by the server.

import type { EngineAnswer, EngineCall, EngineMethod, EngineReply } from './protocol'

type EngineApi = Record<EngineMethod, (json: string) => string> & { version: string; modelVersion: string }

declare const self: DedicatedWorkerGlobalScope
declare const Go: { new (): { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> } }

let ready: Promise<EngineApi> | null = null

// load fetches the engine once. Both files come from the same place, so one deployment serves one engine
// build. wasm_exec.js is Go's own loader: it is not a module and only sets globalThis.Go, but this worker is a
// module worker, where importScripts does not exist, so it is imported for its effect instead.
function load(base: string): Promise<EngineApi> {
  ready ??= (async () => {
    await import(/* @vite-ignore */ `${base}/wasm_exec.js`)
    const go = new Go()
    const source = await WebAssembly.instantiateStreaming(fetch(`${base}/engine.wasm`), go.importObject)
    void go.run(source.instance)
    const api = (self as unknown as { robotsEngine?: EngineApi }).robotsEngine
    if (!api) {
      throw new Error('Сборка движка не отдала методы.')
    }
    return api
  })().catch((err: unknown) => {
    ready = null
    throw err
  })
  return ready
}

self.onmessage = (e: MessageEvent<EngineCall>) => {
  const call = e.data
  const reply = (r: EngineAnswer) => self.postMessage({ id: call.id, ...r } as EngineReply)
  load(call.base)
    .then((api) => {
      const answer = JSON.parse(api[call.method](JSON.stringify(call.request))) as
        | { ok: true; result: unknown }
        | { ok: false; error: string }
      if (!answer.ok) {
        reply({ ok: false, error: answer.error })
        return
      }
      reply({ ok: true, result: answer.result, version: api.version })
    })
    .catch((err: unknown) => {
      reply({ ok: false, error: err instanceof Error ? err.message : 'Движок не запустился.' })
    })
}
