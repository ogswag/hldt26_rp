// Each tab holds a Web Lock on its own queue for as long as it is open. A queue whose lock is free belongs to
// a tab that closed, so another tab of the same person may take it over and send what it never sent.

const name = (projectId: string, owner: string, clientId: string) => `queue:${projectId}:${owner}:${clientId}`

type Locks = {
  request(
    name: string,
    options: { mode?: 'exclusive' | 'shared'; ifAvailable?: boolean; signal?: AbortSignal },
    fn: (lock: unknown) => Promise<unknown>,
  ): Promise<unknown>
}

function locks(): Locks | null {
  const api = (navigator as unknown as { locks?: Locks }).locks
  return api ?? null
}

// available reports whether this browser can tell an open tab from a closed one at all. Without it a queue is
// never taken over: sending someone's edits twice from two live tabs is worse than sending them late.
export function available(): boolean {
  return locks() !== null
}

const held = new Set<string>()

// hold claims this tab's queue until the tab closes. The promise inside never settles, which is how a Web Lock
// is held for a page's lifetime.
export function hold(projectId: string, owner: string, clientId: string): void {
  const api = locks()
  const key = name(projectId, owner, clientId)
  if (!api || held.has(key)) {
    return
  }
  held.add(key)
  void api.request(key, {}, () => new Promise(() => {})).catch(() => {
    held.delete(key)
  })
}

// claim runs take() while holding the queue's lock, and only if no live tab holds it. It answers false when the
// lock was busy, so the caller leaves that queue alone.
export async function claim(projectId: string, owner: string, clientId: string, take: () => Promise<void>): Promise<boolean> {
  const api = locks()
  if (!api) {
    return false
  }
  let taken = false
  await api.request(name(projectId, owner, clientId), { ifAvailable: true }, async (lock) => {
    if (!lock) {
      return
    }
    taken = true
    await take()
  })
  return taken
}
