import { describe, expect, it } from 'vitest'

import type { MapDocument } from '../api/client'

import { preview } from './commands'
import { asMapDocument, checkDeviation, edgeLabel, emptyMap, featureLabel, forSave, metersPerPx, nextName, pointIndex } from './document'

const newId = 'b3a5e0f2-7d7c-4c3e-9d55-0d1f2e3a4b5c'

function base(): MapDocument {
  const doc = emptyMap({ width_px: 800, height_px: 600, source_kind: 'png' })
  doc.layers.points = [
    { id: 'A', kind: 'dock', x: 0, y: 0 },
    { id: 'B', kind: 'task', x: 100, y: 0 },
    { id: newId, kind: 'charger', name: 'C1', x: 100, y: 100 },
  ]
  doc.layers.edges = [
    { id: 'e1', from: 'A', to: 'B', width_m: 3 },
    { id: 'e2', from: 'B', to: newId },
  ]
  doc.layers.obstacles = [{ id: 'o1', kind: 'rack', ring: [{ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 10, y: 10 }] }]
  return doc
}

describe('preview', () => {
  it('moves the dragged point or polygon and nothing else', () => {
    const doc = base()
    const moved = preview(doc, { type: 'updatePoint', id: 'B', patch: { x: 120, y: 5 } })
    expect(moved.layers.points[1]).toEqual({ id: 'B', kind: 'task', x: 120, y: 5 })
    expect(moved.layers.points[0]).toBe(doc.layers.points[0])
    const ring = [{ x: 1, y: 1 }, { x: 9, y: 1 }, { x: 9, y: 9 }]
    expect(preview(doc, { type: 'updatePolygon', layer: 'obstacles', id: 'o1', patch: { ring } }).layers.obstacles[0].ring).toEqual(ring)
    expect(preview(doc, { type: 'deletePoint', id: 'A' })).toBe(doc)
  })
})

describe('document helpers', () => {
  it('labels features by their point code or by their name, never by an internal id', () => {
    const doc = base()
    const points = pointIndex(doc)
    expect(featureLabel(doc.layers.points[0])).toBe('A')
    expect(featureLabel(doc.layers.points[2])).toBe('C1')
    expect(edgeLabel(doc.layers.edges[0], points)).toBe('A - B')
    expect(edgeLabel({ id: newId, from: 'A', to: newId }, points)).toBe('A - C1')
    expect(featureLabel({ id: 'zone-work', name: 'Рабочая область' })).toBe('Рабочая область')
    expect(featureLabel({ id: 'zone-work' })).toBe('без имени')
    expect(nextName(doc, 'C')).toBe('C2')
    expect(nextName(doc, 'e')).toBe('e3')
  })

  it('calibrates from a segment and rejects a zero one', () => {
    expect(metersPerPx({ x1: 0, y1: 0, x2: 30, y2: 40, length_m: 5 })).toBeCloseTo(0.1)
    expect(metersPerPx({ x1: 0, y1: 0, x2: 0, y2: 0, length_m: 10 })).toBeNull()
    expect(checkDeviation({ x1: 0, y1: 0, x2: 100, y2: 0, length_m: 5.2 }, 0.05)).toBeCloseTo(3.846, 2)
  })

  it('cleans the document for checking and reads it back', () => {
    const doc = { ...base(), errors: ['x'], warnings: ['y'] }
    doc.layers.points[1] = { ...doc.layers.points[1], x: 100.123456 }
    const saved = forSave(doc)
    expect(saved.errors).toBeUndefined()
    expect(saved.layers.points[1].x).toBe(100.12)
    expect(asMapDocument(JSON.parse(JSON.stringify(saved)))).not.toBeNull()
    expect(asMapDocument({ schema_version: 'map-v2' })).toBeNull()
    expect(asMapDocument({ schema_version: 'map-v1', calibration: { meters_per_px: 1 }, layers: { zones: [] } })).toBeNull()
  })
})
