const limit = 5

function key(scope: string): string {
  return `search-recent:${scope || 'home'}`
}

// loadRecent lists the ids picked last in this project, newest first. Storage may be off; then there are none.
export function loadRecent(scope: string): string[] {
  try {
    const raw = localStorage.getItem(key(scope))
    const list: unknown = raw ? JSON.parse(raw) : []
    return Array.isArray(list) ? list.filter((v): v is string => typeof v === 'string').slice(0, limit) : []
  } catch {
    return []
  }
}

export function saveRecent(scope: string, id: string): void {
  try {
    const next = [id, ...loadRecent(scope).filter((v) => v !== id)].slice(0, limit)
    localStorage.setItem(key(scope), JSON.stringify(next))
  } catch {
    // NOTE: without storage the search simply has no recent picks.
  }
}
