import { describe, expect, it } from 'vitest'

import { ApiError } from '../api/client'
import { errorKind, type ErrorInfo } from './errorKind'

const api = (status: number, code?: string, requestId?: string) => new ApiError('Текст API.', status, code, [], [], requestId)

describe('errorKind', () => {
  const cases: { name: string; err: unknown; guest?: boolean; want: Partial<ErrorInfo> }[] = [
    { name: 'missing project', err: api(404), want: { kind: 'projectNotFound', status: 404 } },
    { name: 'malformed project id', err: api(400), want: { kind: 'projectNotFound', status: 400 } },
    { name: 'project in the trash', err: api(410, 'project_deleted'), want: { kind: 'trashed', status: 410 } },
    { name: 'other gone', err: api(410), want: { kind: 'projectNotFound' } },
    { name: 'signed out', err: api(401), want: { kind: 'signIn', status: 401 } },
    { name: 'guest opens a saved project', err: api(403), guest: true, want: { kind: 'signIn', status: 403 } },
    { name: 'member without the role', err: api(403, 'forbidden'), want: { kind: 'noAccess', message: 'Текст API.' } },
    { name: 'guest without the role still reads the text', err: api(403, 'forbidden'), guest: true, want: { kind: 'noAccess' } },
    { name: 'server fault keeps the request id', err: api(500, undefined, 'req-1'), want: { kind: 'serverDown', status: 500, requestId: 'req-1' } },
    { name: 'proxy without the API', err: api(503, 'api_unavailable', 'req-2'), want: { kind: 'serverDown', status: 503, requestId: 'req-2' } },
    { name: 'no answer at all', err: new TypeError('Failed to fetch'), want: { kind: 'serverDown' } },
    { name: 'chunk from an old build', err: new TypeError('Failed to fetch dynamically imported module: /assets/Sim-1.js'), want: { kind: 'updated' } },
    { name: 'render crash', err: new Error('x is undefined'), want: { kind: 'crash', message: 'Error: x is undefined' } },
    { name: 'thrown string', err: 'boom', want: { kind: 'crash', message: 'boom' } },
  ]
  for (const c of cases) {
    it(c.name, () => {
      expect(errorKind(c.err, { guest: c.guest })).toMatchObject(c.want)
    })
  }
})
