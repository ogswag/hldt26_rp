export type EntryKind =
  | 'tab'
  | 'section'
  | 'field'
  | 'process'
  | 'processField'
  | 'map'
  | 'result'
  | 'run'
  | 'command'
  | 'project'
  | 'robot'

// SearchEntry is one thing the search can take the user to. id is also the data-search-id of the element on the
// page that the jump reveals, so the index and the page agree without a registry.
export type SearchEntry = {
  id: string
  kind: EntryKind
  label: string
  fullLabel?: string
  // path is where the entry sits, from the tab down: «Объект», «Площадка», «Габариты».
  path: string[]
  note?: string
  aliases?: string[]
  value?: string
  // to is the address to open; a command that runs in place has none.
  to?: string
  // anchor is revealed while the user only looks (arrows): the card of a process whose field opens in a dialog.
  // focusAt takes the focus on confirm when the entry itself is not an element: the panel of a map object.
  anchor?: string
  focusAt?: string
  // section groups the rows the preview shows around the entry.
  section?: string
  order: number
  // closed says why the entry's tab is closed; such an entry is listed but not jumped to.
  closed?: string
  // run acts in place (theme, undo); click names the button the page presses after the jump.
  run?: () => void
  click?: string
  // hint is the preview's one line for a command or a project.
  hint?: string
}

// jumpable tells whether arrowing onto the entry may move the page: commands, other projects and closed tabs wait
// for Enter.
export function jumpable(e: SearchEntry): boolean {
  return e.to !== undefined && !e.closed && e.kind !== 'command' && e.kind !== 'project'
}
