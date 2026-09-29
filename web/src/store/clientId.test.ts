import { describe, expect, it } from 'vitest'

import { tabIds } from './clientId'

// page is the storage and the events of one browser tab: a fresh page load gets the tab's storage as it was left.
function tab(storage = new Map<string, string>()) {
  const kept = {
    getItem: (k: string) => storage.get(k) ?? null,
    setItem: (k: string, v: string) => void storage.set(k, v),
    removeItem: (k: string) => void storage.delete(k),
  }
  const win = new EventTarget()
  const doc = new EventTarget()
  return { storage, ids: tabIds(kept, win, doc), win, doc }
}

describe('client id', () => {
  it('stays the same for the whole life of a page and is not left in the storage', () => {
    const t = tab()
    const id = t.ids()
    expect(t.ids()).toBe(id)
    expect(t.storage.size).toBe(0)
  })

  it('survives a reload: the page hands it over when it goes away', () => {
    const before = tab()
    const id = before.ids()
    before.win.dispatchEvent(new Event('pagehide'))
    const after = tab(before.storage)
    expect(after.ids()).toBe(id)
    expect(after.storage.size).toBe(0)
  })

  it('gives a duplicated tab an id of its own, since a live page keeps nothing in the storage', () => {
    const original = tab()
    const id = original.ids()
    const copy = tab(new Map(original.storage))
    expect(copy.ids()).not.toBe(id)
  })

  it('takes the id back out when the page returns from the cache or from a freeze', () => {
    const t = tab()
    const id = t.ids()
    t.win.dispatchEvent(new Event('pagehide'))
    expect(t.storage.get('robots-client-id')).toBe(id)
    t.win.dispatchEvent(new Event('pageshow'))
    expect(t.storage.size).toBe(0)
    t.doc.dispatchEvent(new Event('freeze'))
    expect(t.storage.get('robots-client-id')).toBe(id)
    t.doc.dispatchEvent(new Event('resume'))
    expect(t.storage.size).toBe(0)
  })

  it('works without a storage', () => {
    const ids = tabIds(null, null, null)
    expect(ids()).toBe(ids())
  })
})
