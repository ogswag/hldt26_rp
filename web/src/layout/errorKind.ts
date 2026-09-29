import { isRouteErrorResponse } from 'react-router-dom'

import { ApiError } from '../api/client'

export type ErrorKind =
  | 'notFound'
  | 'projectNotFound'
  | 'trashed'
  | 'signIn'
  | 'noAccess'
  | 'serverDown'
  | 'updated'
  | 'crash'

export type ErrorInfo = {
  kind: ErrorKind
  status?: number
  requestId?: string
  // message is the server's own text (no access) or the text of a crash, for the report.
  message?: string
}

// A deploy replaces the hashed chunks, so a tab opened before it fails to load the next lazy page.
const staleChunk = /dynamically imported module|Importing a module script failed|ChunkLoadError|Unable to preload CSS/i
// fetch rejects with a TypeError when the request never got an answer.
const noAnswer = /Failed to fetch|NetworkError|Load failed|Network request failed/i

// errorKind sorts an error thrown while loading or drawing a page into the error page that explains it. guest
// matters for a 403 without a code: the API refuses a guest any saved project that way.
export function errorKind(err: unknown, opts: { guest?: boolean } = {}): ErrorInfo {
  if (isRouteErrorResponse(err)) {
    return err.status === 404 ? { kind: 'notFound', status: 404 } : { kind: 'crash', status: err.status }
  }
  if (err instanceof ApiError) {
    const base = { status: err.status, requestId: err.requestId }
    if (err.status === 404 || err.status === 400) {
      return { kind: 'projectNotFound', ...base }
    }
    if (err.status === 410) {
      return { kind: err.code === 'project_deleted' ? 'trashed' : 'projectNotFound', ...base }
    }
    if (err.status === 401 || (err.status === 403 && !err.code && opts.guest)) {
      return { kind: 'signIn', ...base }
    }
    if (err.status === 403) {
      return { kind: 'noAccess', ...base, message: err.message }
    }
    if (err.status >= 500) {
      return { kind: 'serverDown', ...base }
    }
    return { kind: 'crash', ...base, message: err.message }
  }
  if (err instanceof Error && staleChunk.test(err.message)) {
    return { kind: 'updated' }
  }
  if (err instanceof TypeError && noAnswer.test(err.message)) {
    return { kind: 'serverDown' }
  }
  if (err instanceof Error) {
    return { kind: 'crash', message: `${err.name}: ${err.message}` }
  }
  return { kind: 'crash', message: String(err) }
}
