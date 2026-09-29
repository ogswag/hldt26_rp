// What the page and the engine worker say to each other.

export type EngineMethod =
  | 'validateParams'
  | 'match'
  | 'calculate'
  | 'simulateMap'
  | 'checkMap'
  | 'mapTemplate'
  | 'draftHash'
  | 'simChecks'
  | 'variantHashes'

export type EngineCall = {
  id: number
  base: string
  method: EngineMethod
  request: unknown
}

export type EngineAnswer = { ok: true; result: unknown; version: string } | { ok: false; error: string }

export type EngineReply = EngineAnswer & { id: number }
