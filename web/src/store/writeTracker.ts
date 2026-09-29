// Counts the storage writes that are still in flight, so a reload can wait for them.

let writing = 0
const waiters = new Set<() => void>()

// track counts a write until it settles.
export function track<T>(write: Promise<T>): Promise<T> {
  writing++
  const done = () => {
    writing--
    if (writing === 0) {
      for (const wake of [...waiters]) {
        wake()
      }
    }
  }
  write.then(done, done)
  return write
}

// persistenceIdle resolves when no write is in flight, or after maxMs: a stuck write must not hold a reload forever.
export function persistenceIdle(maxMs = 2000): Promise<void> {
  if (writing === 0) {
    return Promise.resolve()
  }
  return new Promise((resolve) => {
    const finish = () => {
      clearTimeout(timer)
      waiters.delete(finish)
      resolve()
    }
    const timer = setTimeout(finish, maxMs)
    waiters.add(finish)
  })
}
