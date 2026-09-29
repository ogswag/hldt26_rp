export type Rect = { left: number; top: number; bottom: number }
export type Size = { width: number; height: number }
export type Placement = { left: number; top: number; maxHeight: number; above: boolean }

const norm = (s: string) => s.toLocaleLowerCase('ru-RU').replaceAll('ё', 'е')

// findByPrefix picks the option for type-to-jump. Repeating one letter cycles through options that start with it.
export function findByPrefix(labels: readonly string[], active: number, typed: string): number {
  const query = norm(typed)
  const n = labels.length
  if (n === 0 || query.trim() === '') {
    return -1
  }
  const repeated = [...query].every((c) => c === query[0])
  const find = (prefix: string, from: number) => {
    for (let k = 0; k < n; k++) {
      const i = (from + k) % n
      if (norm(labels[i]).startsWith(prefix)) {
        return i
      }
    }
    return -1
  }
  const next = Math.max(0, active + 1) % n
  const hit = find(query, query.length > 1 && !repeated ? Math.max(0, active) : next)
  if (hit >= 0 || !repeated) {
    return hit
  }
  return find(query[0], next)
}

// clampLeft keeps a box inside the view, margin from each edge; a box wider than the view starts at the margin.
export function clampLeft(left: number, width: number, viewWidth: number, margin: number): number {
  return Math.max(margin, Math.min(left, viewWidth - margin - width))
}

// placeList opens the list under the anchor, or above it when it does not fit below and there is more room above.
export function placeList(anchor: Rect, list: Size, view: Size, gap: number, margin: number): Placement {
  const below = view.height - margin - anchor.bottom - gap
  const above = anchor.top - gap - margin
  const flip = list.height > below && above > below
  const maxHeight = Math.max(0, Math.min(list.height, flip ? above : below))
  return {
    left: clampLeft(anchor.left, list.width, view.width, margin),
    top: flip ? anchor.top - gap - maxHeight : anchor.bottom + gap,
    maxHeight,
    above: flip,
  }
}

// popoverScale turns viewport coordinates from getBoundingClientRect into the CSS coordinates of a panel. The
// desktop interface uses CSS zoom, while top-layer popovers and positioned panels still receive unzoomed left and top.
export function popoverScale(el: HTMLElement): number {
  const rect = el.getBoundingClientRect()
  const width = el.offsetWidth > 0 ? rect.width / el.offsetWidth : 0
  const height = el.offsetHeight > 0 ? rect.height / el.offsetHeight : 0
  return width > 0 ? width : height > 0 ? height : 1
}

export function popoverSize(el: HTMLElement): Size {
  const rect = el.getBoundingClientRect()
  return { width: rect.width, height: rect.height }
}

export function viewportSize(): Size {
  return { width: window.innerWidth, height: window.innerHeight }
}
