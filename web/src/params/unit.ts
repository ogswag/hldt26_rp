import type { SchemaField } from '../api/client'

// unitOf is the unit shown with a field's value; '-' in the schema means the value has none.
export function unitOf(f: SchemaField): string {
  return f.unit && f.unit !== '-' ? unitText(f.unit) : ''
}

export function unitText(unit: string): string {
  return unit.replaceAll('.', '')
}
