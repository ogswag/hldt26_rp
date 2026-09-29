// The client's picture of a project: what the server confirmed, plus the local transactions it has not.

import { apply, type Change, type Hooks, type Schema, type State, type Tx } from './apply'

export type Pending = {
  tx: Tx
  // The server accepted it with this seq; the transaction leaves pending once confirmed reaches it.
  ackSeq?: number
  // Records as the user meant them, kept to restore them if the server refuses the transaction.
  intended: Change[]
  createdAt: number
}

// view applies pending transactions over confirmed. One that no longer applies (someone deleted its target) is
// left out of the view and stays pending until the server answers. Records nobody touched keep their objects,
// so selectors can compare by reference.
export function view(s: Schema, confirmed: State, pending: readonly Pending[], h: Hooks): State {
  let st = confirmed
  for (const p of pending) {
    const r = apply(s, st, p.tx, h)
    if (r.outcome.status === 'applied') {
      st = r.state
    }
  }
  return st
}

// permissive hooks leave params, named objects and catalog refs to the server.
export const permissive: Hooks = {
  params: () => null,
  object: () => true,
  external: () => true,
}
