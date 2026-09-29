import type { SolutionVariant } from '../api/client'

export type FleetLine = SolutionVariant['fleet'][number]

export function lineKeyOf(f: FleetLine, i: number): string {
  return f.id || `fleet-${i + 1}`
}

// findLine names the variant and the fleet line a search ran for. A variant or a line that is gone gives null.
export function findLine(variants: SolutionVariant[], variantId: string, lineKey: string): SolutionVariant | null {
  const v = variants.find((x) => x.id === variantId)
  return v && v.fleet.some((f, i) => lineKeyOf(f, i) === lineKey) ? v : null
}

export function withLineQuantity(fleet: FleetLine[], lineKey: string, n: number): FleetLine[] {
  return fleet.map((f, i) => (lineKeyOf(f, i) === lineKey ? { ...f, quantity: Math.max(1, n) } : f))
}
