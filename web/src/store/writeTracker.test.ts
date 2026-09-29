import { afterEach, describe, expect, it, vi } from 'vitest'

import { persistenceIdle, track } from './writeTracker'

afterEach(() => {
  vi.useRealTimers()
})

describe('persistenceIdle', () => {
  it('resolves at once when nothing is being written', async () => {
    await expect(persistenceIdle()).resolves.toBeUndefined()
  })

  it('waits for the writes in flight, failed ones included', async () => {
    let finish!: () => void
    let fail!: (e: Error) => void
    const slow = track(new Promise<void>((resolve) => (finish = resolve)))
    const broken = track(new Promise<void>((_, reject) => (fail = reject)))
    broken.catch(() => {})
    let idle = false
    void persistenceIdle().then(() => (idle = true))
    await Promise.resolve()
    expect(idle).toBe(false)
    finish()
    await slow
    await Promise.resolve()
    expect(idle).toBe(false)
    fail(new Error('quota'))
    await broken.catch(() => {})
    await new Promise((r) => setTimeout(r, 0))
    expect(idle).toBe(true)
  })

  it('gives up after the cap when a write never ends', async () => {
    vi.useFakeTimers()
    void track(new Promise<void>(() => {}))
    let idle = false
    void persistenceIdle(2000).then(() => (idle = true))
    await vi.advanceTimersByTimeAsync(1999)
    expect(idle).toBe(false)
    await vi.advanceTimersByTimeAsync(2)
    expect(idle).toBe(true)
  })
})
