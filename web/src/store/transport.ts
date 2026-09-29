// httpTransport is the store's server: the operation endpoints of the API client, with failures sorted by what
// sync should do about them.

import { ApiError, fetchOperations, fetchSnapshot, postTransactions } from '../api/client'
import { SyncError, type SyncErrorKind, type Transport } from './sync'

function kindOf(e: unknown): SyncErrorKind {
  if (!(e instanceof ApiError)) {
    return 'network'
  }
  if (e.status === 409 && e.code === 'schema_version') {
    return 'schema'
  }
  if (e.status === 413) {
    return 'too_large'
  }
  if (e.status === 404 || e.status === 410) {
    return 'gone'
  }
  if (e.status === 403 && e.code === 'forbidden') {
    return 'forbidden'
  }
  // A guest (expired session) or a stale CSRF token: keep the queue and try again after the next login.
  if (e.status === 401 || e.status === 403) {
    return 'auth'
  }
  return 'server'
}

async function call<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn()
  } catch (e) {
    throw new SyncError(kindOf(e), e instanceof Error ? e.message : String(e))
  }
}

export const httpTransport: Transport = {
  snapshot: (projectId) => call(() => fetchSnapshot(projectId)),
  operations: (projectId, after) => call(() => fetchOperations(projectId, after)),
  transactions: (projectId, body) => call(() => postTransactions(projectId, body)),
}
