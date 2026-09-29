import { describe, expect, it } from 'vitest'

import type { State } from '../store/apply'
import { paramFieldEdit } from './econRecords'

function state(params: Record<string, unknown>): State {
  return {
    project: {
      project: { id: 'project', name: 'Склад', object_type: 'warehouse', params, match_selected_ids: [], econ_overrides: null, active_assumption_set_id: null },
    },
  }
}

// Each field saves itself, so one edit must touch one parameter and nothing else.
describe('paramFieldEdit', () => {
  it('writes only the field that changed', () => {
    const edit = paramFieldEdit(state({ area: 20000, ceiling: 10 }), 'area', 31000)
    expect(edit.ops).toEqual([{ op: 'set', coll: 'project', id: 'project', path: 'params.area', value: 31000 }])
  })

  it('says nothing when the value is the one already stored', () => {
    expect(paramFieldEdit(state({ area: 20000 }), 'area', 20000).ops).toEqual([])
  })

  it('writes a field marked unknown as no value', () => {
    expect(paramFieldEdit(state({ power: 250 }), 'power', null).ops).toEqual([
      { op: 'set', coll: 'project', id: 'project', path: 'params.power', value: null },
    ])
  })

  it('treats a cleared field the same as an unknown one, and repeats neither', () => {
    expect(paramFieldEdit(state({ power: 250 }), 'power', undefined).ops).toHaveLength(1)
    expect(paramFieldEdit(state({ power: null }), 'power', undefined).ops).toEqual([])
  })

  it('writes a field the project never had', () => {
    expect(paramFieldEdit(state({}), 'area', 12000).ops).toHaveLength(1)
  })
})
