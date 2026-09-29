import type { Solution } from '../api/client'

export type SourceInfo = {
  url: string | null
  confidence: string | null
  note: string | null
  date: string | null
  // own is false when the value has no entry and the card's source is shown instead.
  own: boolean
}

// sourceOf says where one value of a robot comes from: its own entry, else the source of the card.
export function sourceOf(s: Solution, code: string): SourceInfo {
  const own = s.field_sources?.[code]
  return {
    url: own?.source_url || s.source_url || null,
    confidence: own?.confidence || s.specs.confidence || null,
    note: own?.note || null,
    date: own?.sourced_at || s.specs.sourced_at || null,
    own: own !== undefined,
  }
}
