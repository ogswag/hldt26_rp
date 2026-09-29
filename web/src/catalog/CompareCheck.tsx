// CompareSelect is the robots picked for the comparison: a table row shows a box for each, and a full selection
// keeps only the boxes that are checked.
export type CompareSelect = {
  ids: ReadonlySet<string>
  full: boolean
  toggle: (id: string) => void
}

export function CompareCheck({ id, name, select }: { id: string; name: string; select: CompareSelect }) {
  const checked = select.ids.has(id)
  return (
    <input
      type="checkbox"
      className="compare-check"
      aria-label={`Сравнить: ${name}`}
      checked={checked}
      disabled={!checked && select.full}
      onChange={() => select.toggle(id)}
    />
  )
}
