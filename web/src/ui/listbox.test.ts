import { describe, expect, it } from 'vitest'

import { clampLeft, findByPrefix, placeList } from './listbox'

describe('findByPrefix', () => {
  const labels = ['все', 'склад', 'аэропорт', 'медучреждение', 'Сортировочный центр']

  it('jumps to the next option starting with the letter and cycles on repeats', () => {
    expect(findByPrefix(labels, 0, 'с')).toBe(1)
    expect(findByPrefix(labels, 1, 'сс')).toBe(4)
    expect(findByPrefix(labels, 4, 'ссс')).toBe(1)
    expect(findByPrefix(labels, -1, 'м')).toBe(3)
  })

  it('keeps the current option while the typed prefix still matches it', () => {
    expect(findByPrefix(labels, 1, 'ск')).toBe(1)
    expect(findByPrefix(labels, 1, 'со')).toBe(4)
    expect(findByPrefix(labels, -1, 'аэ')).toBe(2)
  })

  it('ignores case and yo, and reports no match', () => {
    expect(findByPrefix(['Ёмкость', 'Ёмкость 2'], 0, 'е')).toBe(1)
    expect(findByPrefix(['1x', '20x', '60x'], 0, '2')).toBe(1)
    expect(findByPrefix(labels, 0, 'ж')).toBe(-1)
    expect(findByPrefix(labels, 0, 'скл ')).toBe(-1)
    expect(findByPrefix(labels, 0, ' ')).toBe(-1)
    expect(findByPrefix([], -1, 'а')).toBe(-1)
  })
})

describe('placeList', () => {
  const view = { width: 1000, height: 800 }

  it('opens below the anchor when the list fits', () => {
    expect(placeList({ left: 100, top: 100, bottom: 130 }, { width: 200, height: 150 }, view, 4, 8)).toEqual({
      left: 100,
      top: 134,
      maxHeight: 150,
      above: false,
    })
  })

  it('flips above when there is more room there', () => {
    expect(placeList({ left: 100, top: 700, bottom: 730 }, { width: 200, height: 150 }, view, 4, 8)).toEqual({
      left: 100,
      top: 546,
      maxHeight: 150,
      above: true,
    })
  })

  it('shrinks the list to the larger side when neither side fits', () => {
    const small = { width: 1000, height: 300 }
    expect(placeList({ left: 0, top: 120, bottom: 150 }, { width: 200, height: 400 }, small, 4, 8)).toEqual({
      left: 8,
      top: 154,
      maxHeight: 138,
      above: false,
    })
  })

  it('keeps the list inside the viewport horizontally', () => {
    expect(placeList({ left: 900, top: 0, bottom: 30 }, { width: 200, height: 100 }, view, 4, 8).left).toBe(792)
    expect(placeList({ left: 900, top: 0, bottom: 30 }, { width: 1200, height: 100 }, view, 4, 8).left).toBe(8)
  })
})

describe('clampLeft', () => {
  it.each([
    ['fits', 100, 200, 100],
    ['overflows right', 900, 200, 784],
    ['overflows left', -40, 200, 16],
    ['is wider than the view', 300, 1200, 16],
  ])('%s', (_, left, width, want) => {
    expect(clampLeft(left, width, 1000, 16)).toBe(want)
  })
})
