import type {
  CalculateResult,
  EconOverrides,
  ObjectSchema,
  ObjectType,
  ParamsMap,
  ParamValue,
  SchemaField,
  TimeRange,
} from '../api/client'
import { currentModelVersion } from '../econ/modelVersion'
import { asDimensions } from '../params/dimensions'

export function isObjectType(v: string | null | undefined): v is ObjectType {
  return v === 'warehouse' || v === 'airport' || v === 'hospital'
}

export function calcKey(type: ObjectType): string {
  return `guest-calc:${type}`
}

export function fieldList(schema: ObjectSchema): SchemaField[] {
  return schema.groups.flatMap((g) => g.fields)
}

export function defaultsFromSchema(fields: SchemaField[]): ParamsMap {
  const out: ParamsMap = {}
  for (const f of fields) {
    out[f.id] = f.default
  }
  return out
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function isUUID(v: string): boolean {
  return UUID_RE.test(v)
}

export function normalizeClock(raw: string): string | undefined {
  const cut = raw.trim()
  const s = cut.includes('.') ? cut.slice(0, cut.indexOf('.')) : cut
  const parts = s.split(':')
  if (parts.length < 2 || parts.length > 3) {
    return undefined
  }
  const h = Number(parts[0])
  const m = Number(parts[1])
  if (!Number.isInteger(h) || !Number.isInteger(m) || h < 0 || h > 23 || m < 0 || m > 59) {
    return undefined
  }
  if (parts.length === 3) {
    const sec = Number(parts[2])
    if (!Number.isInteger(sec) || sec < 0 || sec > 59) {
      return undefined
    }
  }
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`
}

export function coerceParam(f: SchemaField, v: unknown): ParamValue | undefined {
  if (v === null && f.allow_unknown) {
    return null
  }
  if (v === undefined) {
    return undefined
  }
  if (f.type === 'boolean') {
    return typeof v === 'boolean' ? v : undefined
  }
  if (f.type === 'enum') {
    if (typeof v !== 'string') {
      return undefined
    }
    if (f.options && f.options.length > 0 && !f.options.some((o) => o.value === v)) {
      return undefined
    }
    return v
  }
  if (f.type === 'string') {
    return typeof v === 'string' && v.trim() !== '' ? v : undefined
  }
  if (f.type === 'multi_enum') {
    if (!Array.isArray(v) || !v.every((x) => typeof x === 'string')) {
      return undefined
    }
    if (f.options && f.options.length > 0 && v.some((x) => !f.options?.some((o) => o.value === x))) {
      return undefined
    }
    if (f.required && v.length === 0 && !f.allow_unknown) {
      return undefined
    }
    return v
  }
  if (f.type === 'time_range') {
    if (!v || typeof v !== 'object' || Array.isArray(v)) {
      return undefined
    }
    const o = v as TimeRange
    const start = typeof o.start === 'string' ? normalizeClock(o.start) : undefined
    const end = typeof o.end === 'string' ? normalizeClock(o.end) : undefined
    if (!start || !end) {
      return undefined
    }
    return { start, end }
  }
  if (f.type === 'dimensions') {
    const d = asDimensions(v)
    if (!d) {
      return undefined
    }
    const inRange = (n: number) =>
      Number.isFinite(n) && (f.min === undefined || n >= f.min) && (f.max === undefined || n <= f.max)
    return inRange(d.length) && inRange(d.width) && inRange(d.height) ? d : undefined
  }
  if (f.type === 'number' || f.type === 'integer') {
    if (typeof v !== 'number' || !Number.isFinite(v)) {
      return undefined
    }
    if (f.type === 'integer' && !Number.isInteger(v)) {
      return undefined
    }
    if (f.min !== undefined && v < f.min) {
      return undefined
    }
    if (f.max !== undefined && v > f.max) {
      return undefined
    }
    return v
  }
  return undefined
}

export function sanitizeParams(fields: SchemaField[], stored: ParamsMap): ParamsMap {
  const out = defaultsFromSchema(fields)
  for (const f of fields) {
    const parsed = coerceParam(f, stored[f.id])
    if (parsed !== undefined) {
      out[f.id] = parsed
    }
  }
  return out
}

export function normalizeOverrides(ov?: EconOverrides): EconOverrides {
  if (!ov) {
    return {}
  }
  const out: EconOverrides = {}
  if (ov.price_rub !== undefined) {
    out.price_rub = ov.price_rub
  }
  if (ov.volume_factor !== undefined) {
    out.volume_factor = ov.volume_factor
  }
  if (ov.labor_factor !== undefined) {
    out.labor_factor = ov.labor_factor
  }
  if (ov.fleet_size !== undefined) {
    out.fleet_size = ov.fleet_size
  }
  if (ov.capex_rub !== undefined) {
    out.capex_rub = ov.capex_rub
  }
  if (ov.opex_year_rub !== undefined) {
    out.opex_year_rub = ov.opex_year_rub
  }
  return out
}

export function overridesForRequest(ov?: EconOverrides): EconOverrides | undefined {
  const n = normalizeOverrides(ov)
  if (Object.keys(n).length === 0) {
    return undefined
  }
  return n
}

export function calcFingerprint(
  objectType: ObjectType,
  params: ParamsMap,
  includeIds: string[],
  overrides?: EconOverrides,
): string {
  const sorted: ParamsMap = {}
  for (const k of Object.keys(params).sort()) {
    sorted[k] = params[k]
  }
  return JSON.stringify({
    object_type: objectType,
    params: sorted,
    include_ids: [...includeIds].sort(),
    overrides: normalizeOverrides(overrides),
  })
}

type StoredCalc = {
  fingerprint: string
  result: CalculateResult
}

function isCalculateResult(v: unknown): v is CalculateResult {
  if (!v || typeof v !== 'object' || Array.isArray(v)) {
    return false
  }
  const o = v as Record<string, unknown>
  return typeof o.model_version === 'string' && Array.isArray(o.scenarios)
}

export function loadGuestCalc(type: ObjectType, fingerprint: string): CalculateResult | null {
  try {
    const raw = localStorage.getItem(calcKey(type))
    if (!raw) {
      return null
    }
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return null
    }
    const obj = parsed as Record<string, unknown>
    if (obj.fingerprint !== fingerprint || !isCalculateResult(obj.result) || obj.result.model_version !== currentModelVersion) {
      return null
    }
    return obj.result
  } catch {
    return null
  }
}

export function saveGuestCalc(type: ObjectType, fingerprint: string, result: CalculateResult): void {
  const payload: StoredCalc = { fingerprint, result }
  try {
    localStorage.setItem(calcKey(type), JSON.stringify(payload))
  } catch {
    return
  }
}

