import type { CalculateResult, MatchItem } from '../api/client'

// suggestedItem is the robot the calculation suggests, with its checks and its own buy case.
export function suggestedItem(result: CalculateResult | null | undefined): MatchItem | null {
  const best = result?.match?.best
  if (!best) {
    return null
  }
  return result.match.items.find((i) => i.solution_id === best) ?? null
}

// suggestionState says whether a suggestion can run in the simulation: it needs a count, which only a priced
// robot gets.
export function suggestionState(result: CalculateResult | null | undefined): 'ready' | 'no_count' | 'none' {
  const item = suggestedItem(result)
  if (!item) {
    return 'none'
  }
  return item.estimate && item.estimate.fleet_size > 0 ? 'ready' : 'no_count'
}
