import type { Dimensions } from '../api/client'

export const dimensionParts = [
  { key: 'length', label: 'Длина' },
  { key: 'width', label: 'Ширина' },
  { key: 'height', label: 'Высота' },
] as const

// parseDimensions reads "1200x800x1600"; the separator may also be X, *, the multiplication sign or Cyrillic х.
export function parseDimensions(text: string): Dimensions | null {
  const parts = text.split(/[xX*\u00d7\u0445\u0425]/)
  if (parts.length !== 3) {
    return null
  }
  const nums = parts.map((p) => {
    const t = p.trim().replace(',', '.')
    return /^\d+(\.\d+)?$/.test(t) ? Number(t) : NaN
  })
  if (nums.some((n) => !(n > 0))) {
    return null
  }
  return { length: nums[0], width: nums[1], height: nums[2] }
}

// asDimensions reads a stored value: the object, or the string that projects kept before the object existed.
// NOTE: a part being typed is NaN in the form draft; it comes back as it is, and the check refuses it.
export function asDimensions(v: unknown): Dimensions | null {
  if (typeof v === 'string') {
    return parseDimensions(v)
  }
  if (!v || typeof v !== 'object' || Array.isArray(v)) {
    return null
  }
  const o = v as Record<string, unknown>
  const keys = Object.keys(o)
  if (keys.length !== 3 || !dimensionParts.every((p) => typeof o[p.key] === 'number')) {
    return null
  }
  return { length: o.length as number, width: o.width as number, height: o.height as number }
}

export function formatDimensions(d: Dimensions): string {
  return `${d.length}x${d.width}x${d.height}`
}
