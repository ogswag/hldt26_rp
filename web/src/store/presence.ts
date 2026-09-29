// Who else has the project open and what they are editing. The server keeps the list; this page reports itself
// every 15 seconds and says goodbye when it closes.

import { postPresence } from '../api/client'

export type Selection = { coll: string; id: string } | null

export type Presence = {
  client_id: string
  user_id: string
  email: string
  route: string
  selection: Selection
}

// Person is one other human in the project, with the colour their marks carry.
export type Person = {
  userId: string
  email: string
  initials: string
  color: number
  selections: Selection[]
}

const REPORT_EVERY = 15_000
export const COLORS = 4

// initials makes a two-letter mark out of an email: "anna.petrova@..." becomes "АП".
export function initials(email: string): string {
  const local = email.split('@')[0] ?? email
  const parts = local.split(/[._-]+/).filter(Boolean)
  const letters = parts.length > 1 ? [parts[0][0], parts[1][0]] : [local[0] ?? '?', local[1] ?? '']
  return letters.join('').toUpperCase()
}

// colorOf keeps one colour per person for as long as the page is open.
export function colorOf(userId: string): number {
  let h = 0
  for (let i = 0; i < userId.length; i++) {
    h = (h * 31 + userId.charCodeAt(i)) % 1_000_003
  }
  return h % COLORS
}

// people groups the pages of the project by person, leaving out every page of this user: their other tabs are
// not a second person.
export function people(list: readonly Presence[], selfUserId: string): Person[] {
  const out = new Map<string, Person>()
  for (const p of list) {
    if (p.user_id === selfUserId) {
      continue
    }
    const person = out.get(p.user_id) ?? {
      userId: p.user_id,
      email: p.email,
      initials: initials(p.email),
      color: colorOf(p.user_id),
      selections: [],
    }
    if (p.selection) {
      person.selections.push(p.selection)
    }
    out.set(p.user_id, person)
  }
  return [...out.values()].sort((a, b) => (a.email < b.email ? -1 : a.email > b.email ? 1 : 0))
}

export type PresenceOptions = {
  clientId: string
  userId: string
  report?: (body: { client_id: string; route: string; selection: Selection; leave?: boolean }, keepalive?: boolean) => Promise<void>
  now?: () => number
}

// Tracker holds the project's presence list, tells the pages about it and reports this page.
export class Tracker {
  private readonly projectId: string
  private readonly opts: PresenceOptions
  private list: readonly Presence[] = []
  private others: Person[] = []
  private readonly listeners = new Set<() => void>()
  private route = ''
  private selection: Selection = null
  private timer: ReturnType<typeof setInterval> | null = null
  private stopped = false

  constructor(projectId: string, opts: PresenceOptions) {
    this.projectId = projectId
    this.opts = opts
  }

  getState = (): Person[] => this.others

  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn)
    return () => {
      this.listeners.delete(fn)
    }
  }

  // onEvent takes the presence list from the project stream. The stream sends {"list": [...]}.
  onEvent(data: unknown): void {
    const list = Array.isArray(data) ? data : (data as { list?: unknown } | null)?.list
    this.list = Array.isArray(list) ? (list as Presence[]) : []
    const next = people(this.list, this.opts.userId)
    if (JSON.stringify(next) !== JSON.stringify(this.others)) {
      this.others = next
      for (const fn of this.listeners) {
        fn()
      }
    }
  }

  // at reports which page of the project this tab is on; it repeats the report so the record does not expire.
  at(route: string): void {
    if (route === this.route) {
      return
    }
    this.route = route
    if (route === '') {
      return
    }
    if (this.timer === null) {
      this.timer = setInterval(() => this.send(false), REPORT_EVERY)
    }
    this.send(false)
  }

  // select reports what this tab has selected on its page. Until the page is known the choice waits for it.
  select(selection: Selection): void {
    if (JSON.stringify(selection) === JSON.stringify(this.selection)) {
      return
    }
    this.selection = selection
    if (this.route !== '') {
      this.send(false)
    }
  }

  // refresh sends the current place again, for a page that comes back from the cache or from hiding.
  refresh(): void {
    if (this.route !== '') {
      this.send(false)
    }
  }

  // leave tells the others this page is gone; it must outlive the page, so it goes with keepalive.
  leave(): void {
    if (this.route === '') {
      return
    }
    this.send(true, true)
  }

  stop(): void {
    this.stopped = true
    if (this.timer !== null) {
      clearInterval(this.timer)
      this.timer = null
    }
    this.leave()
  }

  private send(leave: boolean, keepalive = false): void {
    if (this.stopped && !leave) {
      return
    }
    const report = this.opts.report ?? ((body, ka) => postPresence(this.projectId, body, ka))
    void report({ client_id: this.opts.clientId, route: this.route, selection: this.selection, ...(leave ? { leave } : {}) }, keepalive).catch(
      () => {},
    )
  }
}
