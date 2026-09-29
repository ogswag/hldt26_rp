import { describe, expect, it } from 'vitest'

import { dueForUpdateCheck, isBehind, isTextEntry, mayReloadForChunk, safeToReload } from './updateRules'

describe('safeToReload', () => {
  it.each([
    [{ typing: false, dialog: false, holds: 0 }, true],
    [{ typing: true, dialog: false, holds: 0 }, false],
    [{ typing: false, dialog: true, holds: 0 }, false],
    [{ typing: false, dialog: false, holds: 1 }, false],
    [{ typing: true, dialog: true, holds: 2 }, false],
  ])('%j is %s', (state, want) => {
    expect(safeToReload(state)).toBe(want)
  })
})

describe('isTextEntry', () => {
  it('counts fields that take text and editable regions', () => {
    expect(isTextEntry('INPUT', 'text', false)).toBe(true)
    expect(isTextEntry('input', undefined, false)).toBe(true)
    expect(isTextEntry('INPUT', 'number', false)).toBe(true)
    expect(isTextEntry('TEXTAREA', undefined, false)).toBe(true)
    expect(isTextEntry('DIV', undefined, true)).toBe(true)
  })

  it('leaves out focus on a checkbox, a button or the page itself', () => {
    expect(isTextEntry('INPUT', 'checkbox', false)).toBe(false)
    expect(isTextEntry('INPUT', 'radio', false)).toBe(false)
    expect(isTextEntry('BUTTON', undefined, false)).toBe(false)
    expect(isTextEntry('BODY', undefined, false)).toBe(false)
  })
})

describe('isBehind', () => {
  const shell = ['/index.html', '/assets/index-new.js', '/assets/index-new.css']

  it('is behind when the worker no longer carries the page\'s entry chunk', () => {
    expect(isBehind('/assets/index-old.js', shell)).toBe(true)
  })

  it('is not behind when the entry chunk is in the shell, as after the first install or a reload', () => {
    expect(isBehind('/assets/index-new.js', shell)).toBe(false)
  })
})

describe('dueForUpdateCheck', () => {
  it('checks at once the first time and then once a minute', () => {
    expect(dueForUpdateCheck(null, 1000)).toBe(true)
    expect(dueForUpdateCheck(1000, 30_000)).toBe(false)
    expect(dueForUpdateCheck(1000, 61_000)).toBe(true)
  })
})

describe('mayReloadForChunk', () => {
  it('allows the first reload and refuses another inside 30 seconds', () => {
    expect(mayReloadForChunk(null, 5000)).toBe(true)
    expect(mayReloadForChunk(5000, 20_000)).toBe(false)
    expect(mayReloadForChunk(5000, 36_000)).toBe(true)
  })

  it('allows a reload when the kept time is in the future, as after a clock change', () => {
    expect(mayReloadForChunk(100_000, 5000)).toBe(true)
  })
})
