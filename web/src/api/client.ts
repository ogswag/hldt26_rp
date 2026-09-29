import { getSession, setSession } from '../auth/session'
import { holdReload } from '../offline/serviceWorker'
import { demoAsset, demoMode, DemoNoServer } from './demo'
import type { State } from '../store/apply'
import type { BatchBody, BatchResponse, OperationsPage, SnapshotResponse } from '../store/sync'

export type ObjectType = 'warehouse' | 'airport' | 'hospital'

export type SolutionSpecs = {
  payload_kg: number | null
  mass_kg: number | null
  length_mm: number | null
  width_mm: number | null
  height_mm: number | null
  speed_mps: number | null
  endurance_h: number | null
  charge_min: number | null
  nav_type: string | null
  pos_accuracy_mm: number | null
  min_aisle_mm: number | null
  turn_radius_mm: number | null
  temp_min_c: number | null
  temp_max_c: number | null
  lifetime_years: number | null
  service_pct_year: number | null
  confidence: string | null
  sourced_at: string | null
}

export type DataQuality = {
  status: string
  missing: string[]
  reasons: string[]
}

// FieldSource says where one value of a robot comes from; a value with no entry has the source of the card.
export type FieldSource = {
  source_url?: string
  confidence?: string
  note?: string
  sourced_at?: string
}

export type Solution = {
  id: string
  name: string
  vendor: string | null
  kind: string | null
  subtype: string | null
  status: string | null
  industry: string | null
  scenario: string | null
  // field_sources are the sources of single values by field code.
  field_sources: Record<string, FieldSource>
  // modification names the row of a robot the catalog lists for several industries; null for the others.
  modification: string | null
  price_rub: number | null
  source_url: string | null
  // family is the robot group of the catalog («Тип»); null reads as «Другое».
  family: string | null
  description: string | null
  object_types: string[] | null
  region: string | null
  ugt: number | null
  market: number | null
  // archived_at is set for a robot in the archive; only admins list those.
  archived_at: string | null
  image_sha: string | null
  specs: SolutionSpecs
  data_quality: DataQuality
}

export type CatalogQuery = {
  q?: string
  kind?: string
  subtype?: string
  vendor?: string
  object_type?: string
  min_payload_kg?: number
  max_width_mm?: number
  sort?: 'name' | 'price_rub' | 'payload_kg'
  // archived include lists archived robots too; admins only.
  archived?: 'include'
  limit?: number
  offset?: number
}

export type SolutionsResponse = {
  items: Solution[]
  total?: number
  limit?: number
  offset?: number
}

export type DictionaryEntry = {
  kind: string
  code: string
  label: string
  object_type?: string
}

export type ObjectTypeInfo = {
  type: ObjectType
  label: string
}

export type FieldType = 'number' | 'integer' | 'string' | 'boolean' | 'enum' | 'multi_enum' | 'time_range' | 'dimensions'

export type EnumOption = {
  value: string
  label: string
}

export type TimeRange = {
  start: string
  end: string
}

// Dimensions is a box size in the field unit (mm).
export type Dimensions = {
  length: number
  width: number
  height: number
}

export type ParamValue = number | string | boolean | string[] | TimeRange | Dimensions | null

export type SchemaField = {
  id: string
  label: string
  // short is the one-line label the form shows; absent when label already fits.
  short?: string
  unit: string
  type: FieldType
  required: boolean
  allow_unknown?: boolean
  default: ParamValue
  min?: number
  max?: number
  note?: string
  help?: string
  options?: EnumOption[]
  aliases?: string[]
}

export type SchemaGroup = {
  id: string
  label: string
  tab: string
  fields: SchemaField[]
}

// SchemaTab is one page of the object form. Key tabs are reviewed before the first calculation.
export type SchemaTab = {
  id: string
  label: string
  key: boolean
}

export type ObjectSchema = {
  type: ObjectType
  label: string
  tabs: SchemaTab[]
  groups: SchemaGroup[]
}

export type ParamsMap = Record<string, ParamValue>

// EngineCatalog is the catalog as the browser engine takes it. The page treats its rows as opaque: they come from
// the API and go straight into the engine.
export type EngineCatalog = {
  content_sha256: string
  candidates: unknown[]
  robots: unknown[]
}

export function fetchCatalogBundle(): Promise<EngineCatalog> {
  return request<EngineCatalog>('/api/catalog/bundle')
}

export type ProcessDemand = {
  units_per_day?: number
  unit?: string
  units_per_job?: number
}

export type ProcessSLA = {
  max_wait_min?: number
  max_cycle_min?: number
  priority?: number
}

export type ProcessDurations = {
  load_s?: number
  unload_s?: number
  travel_s?: number
}

export type ProcessStaff = {
  headcount?: number
  role?: string
}

export type Process = {
  id?: string
  code: string
  name: string
  task_type: string
  is_baseline: boolean
  demand: ProcessDemand
  sla: ProcessSLA
  point_ids?: string[]
  durations: ProcessDurations
  baseline_staff: ProcessStaff
  sort_order: number
}

export type FleetItem = {
  id?: string
  solution_id: string | null
  quantity: number
  task_codes?: string[]
  price_override_rub: number | null
  price_override_reason?: string
  sort_order: number
}

export type Financing = {
  id?: string
  kind: 'buy' | 'raas'
  tariff: 'fixed' | 'variable' | 'mixed' | null
  assumptions?: Record<string, unknown>
}

export type SolutionVariant = {
  id?: string
  name: string
  status: 'draft' | 'ready'
  notes?: string
  sort_order: number
  fleet: FleetItem[]
  financing: Financing[]
}

export type SharedCost = {
  id?: string
  code: string
  label: string
  bucket: 'capex' | 'opex'
  rub: number
  sort_order: number
}

export type AssumptionSet = {
  id?: string
  name: string
  is_active: boolean
  vat_rate: number
  prices_include_vat: boolean
  vat_recoverable: boolean
  labor_cash_share: number
  discount_rate: number
  utilization?: number | null
  availability?: number | null
  reserve?: number | null
  service_share?: number | null
  delivery_share?: number | null
  comm_rub_per_robot_year?: number | null
  technician_wage_month_rub?: number | null
  sort_order: number
}

export type NormValue = {
  value: number
  set: boolean
}

export type ConfidenceLevel = 'preliminary' | 'configured' | 'calibrated' | 'validated'

export type RunKind = 'calculation' | 'simulation'

export type RunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled'

export type CalculationRunListItem = {
  id: string
  project_id: string
  project_version_id: string
  input_hash: string
  match_version: string
  econ_version: string
  sim_version: string
  seed: number
  status: RunStatus
  created_at: string | null
  version_no: number
  confidence_level: ConfidenceLevel
  is_current: boolean
  stale_vs_draft: boolean
}

export type RunMeta = {
  id: string
  kind: RunKind
  status: RunStatus
  project_id: string
  project_version_id: string
  version_no: number
  input_hash: string
  match_version: string
  econ_version: string
  sim_version: string
  seed: number
  confidence_level: ConfidenceLevel
  created_at: string | null
  is_current: boolean
}

export type CalculationBrief = {
  solution_name?: string | null
  verification_flag?: boolean | null
  variant_names?: string[]
  best_payback_years?: number | null
}

export type SimulationRunBrief = {
  variant_name?: string | null
  map_source?: 'project' | 'template' | null
  verdict?: 'pass' | 'fail' | null
  verdict_text?: string | null
  throughput_per_h?: number | null
  violation_rate?: number | null
}

export type RunHistoryItem = RunMeta & {
  job_id: string | null
  stale_vs_draft: boolean
  brief: CalculationBrief & SimulationRunBrief
}

export type RunHistory = {
  items: RunHistoryItem[]
  input_hash: string
  current_run_id: string | null
}

export type CalculationResponse = CalculateResult & {
  run_id?: string
  input_hash?: string
  project_id?: string
  project_name?: string
  stale_vs_draft?: boolean
  run?: RunMeta
}

export type ExportFormat = 'pdf' | 'xlsx'

export type ProjectDraftDocument = {
  schema_version: string
  object_type: string
  params: ParamsMap
  processes: Process[]
  variants: SolutionVariant[]
  shared_costs?: SharedCost[]
  assumption_sets?: AssumptionSet[]
  active_assumption_set_id?: string
  map: MapDocument | null
  match_selected_ids: string[]
  econ_overrides?: EconOverrides | null
}

export type ProjectSnapshot = {
  schema_version: string
  project_id: string
  version_no: number
  name: string
  object_type: string
  draft: ProjectDraftDocument
  input_hash: string
  match_version: string
  econ_version: string
  sim_version: string
  confidence_level: ConfidenceLevel
}

export type ProjectCreate = {
  name: string
  object_type: ObjectType
  params?: ParamsMap
}

export type ScenarioResult = {
  kind: string
  solution_id: string | null
  fleet_size: number | null
  capex_rub: number | null
  opex_year_rub: number | null
  annual_effect_rub: number | null
  payback_years: number | null
  payback_band?: string
  roi_pct: number | null
  tco_rub: number | null
  accounting_effect_rub?: number | null
  labor_cash_saved_rub?: number | null
  discounted_payback_years?: number | null
  npv_rub?: number | null
  irr_pct?: number | null
  cash_flow?: FlowRow[]
  variant_id?: string
  variant_name?: string
  tariff?: string
  price_source?: string
}

export type FlowRow = {
  year: number
  effect_rub: number
  battery_rub?: number
  replacement_rub?: number
  net_rub: number
  cumulative_rub: number
}

export type EconRisk = {
  id: string
  level: string
  text: string
}

export type ScoreParts = {
  fit: number
  process_match: number
  data_quality: number
  price_band: number
}

export type MatchStep = {
  rule_id: string
  kind: 'hard' | 'soft' | 'missing_evidence' | string
  outcome: 'pass' | 'fail' | 'unknown' | string
  task_code?: string
  capability_code?: string
  object_field?: string
  object_value: number | null
  object_unit?: string
  solution_field?: string
  solution_value: number | null
  solution_unit?: string
  source_url?: string | null
  confidence?: string | null
  text: string
}

export type MatchItem = {
  solution_id: string
  name: string
  vendor: string | null
  kind: string | null
  subtype: string | null
  family?: string
  price_rub: number | null
  status: string
  reasons: string[]
  hard?: MatchStep[]
  soft?: MatchStep[]
  missing_evidence?: MatchStep[]
  explanation?: MatchStep[]
  score_parts: ScoreParts
  score: number
  forced: boolean
  // estimate is the robot's own buy case, filled by a calculation for robots that are not excluded.
  estimate?: MatchEstimate
}

export type MatchEstimate = {
  fleet_size: number
  capex_rub: number
  // payback_years is null when the robot does not pay back.
  payback_years: number | null
  // process_codes are the project processes the robot serves in this case.
  process_codes?: string[]
}

export type MatchOutput = {
  weights: ScoreParts
  selected_ids: string[]
  items: MatchItem[]
  match_version?: string
  catalog_content_sha256?: string
  task_codes?: string[]
  // best is the suggested robot and best_why says why it leads.
  best?: string
  best_why?: string[]
}

export type EconLine = {
  scenario: string
  bucket: string
  id: string
  label: string
  rub: number
  note?: string
}

export type EconFormula = {
  id: string
  text: string
  unit: string
  // mathml is the same formula as MathML markup from the API.
  mathml?: string
}

export type EconOverride = {
  field: string
  value: number
  computed: number
  flag: boolean
}

export type EconSensitivity = {
  variant_id?: string
  variant_name?: string
  param: string
  delta_pct: number
  buy: ScenarioResult
  raas: ScenarioResult
}

// EconSimCheck compares what a simulation delivers for one variant with what the economics counts on.
export type EconSimCheck = {
  variant_id: string
  variant_name: string
  // model is the way of checking: the run on the project map, or the quick check without a map.
  model: 'map' | 'quick'
  value: number
  flag: boolean
  text?: string
  run_id?: string
  stale?: boolean
}

export type EconShared = {
  fleet_size: number
  chargers?: number
  throughput: number
  peak_ops: number
  unit: string
  work_kind: string
}

export type EconOverrides = {
  price_rub?: number
  volume_factor?: number
  labor_factor?: number
  fleet_size?: number
  capex_rub?: number
  opex_year_rub?: number
}

export type CalculateResult = {
  model_version: string
  object_type: string
  seed: number
  scenarios: ScenarioResult[]
  match: MatchOutput
  assumptions: string[]
  verification_flag: boolean
  formulas?: EconFormula[]
  units?: Record<string, string>
  sources?: string[]
  breakdown?: EconLine[]
  overrides?: EconOverride[]
  sensitivity?: EconSensitivity[]
  shared?: EconShared
  horizon_years?: number
  solution_name?: string
  risks?: EconRisk[]
  interpretation?: string[]
  sim?: SimSummary
  variants?: VariantResult[]
  origins?: MetricOrigin[]
  assumption_set?: AssumptionView
  sim_checks?: EconSimCheck[]
}

export type MetricOrigin = {
  metric: string
  source: string
  ref?: string
  note: string
}

export type AssumptionView = {
  id?: string
  name: string
  vat_rate: number
  prices_include_vat: boolean
  vat_recoverable: boolean
  labor_cash_share: number
  discount_rate?: number
  utilization?: NormValue
  availability?: NormValue
  reserve?: NormValue
  service_share?: NormValue
  delivery_share?: NormValue
  comm_rub_per_robot_year?: NormValue
  technician_wage_month_rub?: NormValue
}

export type FleetKPI = {
  solution_id: string
  name: string
  quantity: number
  catalog_price_rub: number | null
  project_price_rub: number | null
  cash_price_rub: number
  lifetime_years?: number
  lifetime_assumed?: boolean
  price_source: string
  price_override_reason?: string
  work_kind: string
  task_codes?: string[]
}

export type VariantResult = {
  variant_id: string
  name: string
  fleet: FleetKPI[]
  chargers?: number
  shared_costs?: EconLine[]
  scenarios: ScenarioResult[]
}

export type SimSummary = {
  throughput: number
  econ_throughput: number
  divergence: number
  delivered_ops_h: number
  queue_wait_s: number
  charge_share: number
  simulated_s: number
  bottleneck: string
  unit: string
  assumptions?: string[]
}

export type Project = {
  id: string
  name: string
  object_type: string
  params: ParamsMap | Record<string, never>
  processes?: Process[]
  variants?: SolutionVariant[]
  shared_costs?: SharedCost[]
  assumption_sets?: AssumptionSet[]
  active_assumption_set_id?: string
  map?: MapDocument | null
  match_selected_ids?: string[]
  econ_overrides?: EconOverrides | null
  results: CalculateResult | null
  model_version: string
  calc_seed: number | null
  created_at: string | null
  input_hash?: string
  current_run_id?: string | null
  stale?: boolean
  // access is the caller's role in the project.
  access?: 'owner' | 'editor' | 'viewer'
}

export type AuthUser = {
  id: string
  email: string
  role: string
}

export type AuthResponse = {
  user: AuthUser
  csrf_token: string
}

export type ProjectListItem = {
  id: string
  name: string
  object_type: string
  model_version: string
  calc_seed: number | null
  created_at: string | null
  has_results: boolean
  stale?: boolean
  input_hash?: string
  current_run_id?: string | null
}

export type FieldError = {
  field: string
  message: string
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string | undefined
  readonly details: FieldError[]
  readonly issues: MapIssue[]
  // requestId names the request in the server log; error pages show it for server faults.
  readonly requestId: string | undefined

  constructor(
    message: string,
    status: number,
    code: string | undefined,
    details: FieldError[],
    issues: MapIssue[] = [],
    requestId?: string,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
    this.issues = issues
    this.requestId = requestId
  }
}

type ErrorJSON = {
  error?: string
  code?: string
  details?: FieldError[]
  issues?: MapIssue[]
  request_id?: string
}

export const serverDownText = 'Сервер не отвечает. Повторите через минуту.'

async function readError(res: Response): Promise<ApiError> {
  let body: ErrorJSON = {}
  try {
    body = (await res.json()) as ErrorJSON
  } catch {
    body = {}
  }
  const details = Array.isArray(body.details) ? body.details : []
  const issues = Array.isArray(body.issues) ? body.issues : []
  const down = res.status === 502 || res.status === 503 || res.status === 504
  const msg =
    body.error ||
    (down ? serverDownText : `Запрос не выполнен (код ${res.status}). Проверьте подключение к серверу и повторите.`)
  const requestId = body.request_id || res.headers.get('X-Request-Id') || undefined
  return new ApiError(msg, res.status, body.code, details, issues, requestId)
}

function unsafeMethod(init: RequestInit): boolean {
  const m = (init.method ?? 'GET').toUpperCase()
  return m !== 'GET' && m !== 'HEAD'
}

async function errorCodeOf(res: Response): Promise<string | undefined> {
  try {
    const body = (await res.clone().json()) as ErrorJSON
    return body.code
  } catch {
    return undefined
  }
}

let refreshing: Promise<boolean> | null = null

// refreshSession reads the cookie's user and CSRF token from /api/auth/me. Concurrent callers share one request.
export function refreshSession(): Promise<boolean> {
  refreshing ??= fetch('/api/auth/me', { cache: 'no-store', credentials: 'same-origin', headers: { Accept: 'application/json' } })
    .then(async (res) => {
      if (!res.ok) {
        setSession(null)
        return false
      }
      const body = (await res.json()) as AuthResponse
      setSession({ user: body.user, csrfToken: body.csrf_token })
      return true
    })
    .catch(() => false)
    .finally(() => {
      refreshing = null
    })
  return refreshing
}

// sessionFetch sends the cookie and, on writes, the CSRF token. An expired session is retried as a guest,
// a stale CSRF token once after /api/auth/me (another tab may have signed in again).
async function sessionFetch(path: string, init: RequestInit, retried = false): Promise<Response> {
  const csrf = getSession()?.csrfToken
  const res = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      ...(csrf && unsafeMethod(init) ? { 'X-CSRF-Token': csrf } : {}),
      ...init.headers,
    },
  })
  if (retried || (res.status !== 401 && res.status !== 403)) {
    return res
  }
  const code = await errorCodeOf(res)
  if (code === 'session_expired') {
    setSession(null)
    return sessionFetch(path, init, true)
  }
  if (code === 'csrf' && (await refreshSession())) {
    return sessionFetch(path, init, true)
  }
  return res
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  if (demoMode) {
    return demoRequest<T>(path, init)
  }
  const res = await sessionFetch(path, {
    ...init,
    cache: init?.cache ?? 'no-store',
    headers: {
      Accept: 'application/json',
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  if (!res.ok) {
    throw await readError(res)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

// demoRequest answers from the files the demo build carries. A write, or a read of something only an account
// has, is refused here rather than sent nowhere.
async function demoRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const asset = init?.method && init.method !== 'GET' ? null : demoAsset(path)
  if (!asset) {
    throw new ApiError(DemoNoServer, 501, 'demo', [])
  }
  const res = await fetch(asset, { headers: { Accept: 'application/json' } })
  if (!res.ok) {
    throw new ApiError(DemoNoServer, res.status, 'demo', [])
  }
  return (await res.json()) as T
}

function filenameFromDisposition(v: string | null, fallback: string): string {
  if (!v) {
    return fallback
  }
  const m = /filename="([^"]+)"/.exec(v)
  if (m?.[1]) {
    return m[1]
  }
  return fallback
}

async function downloadFile(path: string, init: RequestInit, fallbackName: string): Promise<void> {
  // A reload for a new build must not cut the file off.
  const release = holdReload()
  try {
    const res = await sessionFetch(path, {
      ...init,
      headers: {
        Accept: 'application/pdf, application/vnd.openxmlformats-officedocument.spreadsheetml.sheet, application/json',
        ...(init.body ? { 'Content-Type': 'application/json' } : {}),
        ...init.headers,
      },
    })
    if (!res.ok) {
      throw await readError(res)
    }
    const blob = await res.blob()
    const name = filenameFromDisposition(res.headers.get('Content-Disposition'), fallbackName)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = name
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } finally {
    release()
  }
}

export function fetchSolutions(query?: CatalogQuery): Promise<SolutionsResponse> {
  const p = new URLSearchParams()
  if (query?.q) p.set('q', query.q)
  if (query?.kind) p.set('kind', query.kind)
  if (query?.subtype) p.set('subtype', query.subtype)
  if (query?.vendor) p.set('vendor', query.vendor)
  if (query?.object_type) p.set('object_type', query.object_type)
  if (query?.min_payload_kg != null) p.set('min_payload_kg', String(query.min_payload_kg))
  if (query?.max_width_mm != null) p.set('max_width_mm', String(query.max_width_mm))
  if (query?.sort) p.set('sort', query.sort)
  if (query?.limit != null) p.set('limit', String(query.limit))
  if (query?.offset != null) p.set('offset', String(query.offset))
  if (query?.archived) p.set('archived', query.archived)
  const qs = p.toString()
  return request<SolutionsResponse>(`/api/solutions${qs ? `?${qs}` : ''}`)
}

export function fetchSolution(id: string): Promise<Solution> {
  return request<Solution>(`/api/solutions/${id}`)
}

export type CatalogFieldKind = 'id' | 'text' | 'number' | 'choice' | 'choices' | 'url' | 'date'

// CatalogField is one catalog field as the admin edits it and catalog files carry it. Values are in stored
// units: a percent field holds a share (0,05 for 5%).
export type CatalogField = {
  code: string
  label: string
  unit?: string
  kind: CatalogFieldKind
  group: 'about' | 'offer' | 'specs' | 'file'
  choices?: { code: string; label: string }[]
  min?: number
  max?: number
  integer?: boolean
  percent?: boolean
  max_len?: number
  required?: boolean
  editable: boolean
}

export type CatalogDictionaries = {
  tasks: DictionaryEntry[]
  capabilities: DictionaryEntry[]
  fields?: CatalogField[]
}

export function fetchCatalogDictionaries(): Promise<CatalogDictionaries> {
  return request('/api/catalog/dictionaries')
}

// solutionImagePath is the robot's photo, versioned by its hash so a new photo is never served from a cache.
export function solutionImagePath(s: Pick<Solution, 'id' | 'image_sha'>): string | null {
  if (!s.image_sha) {
    return null
  }
  const path = `/api/solutions/${s.id}/image`
  return demoMode ? demoAsset(path) : `${path}?v=${s.image_sha.slice(0, 16)}`
}

export function fetchObjectTypes(): Promise<{ items: ObjectTypeInfo[] }> {
  return request<{ items: ObjectTypeInfo[] }>('/api/object-types')
}

export function fetchObjectSchema(type: ObjectType): Promise<ObjectSchema> {
  return request<ObjectSchema>(`/api/object-types/${type}/schema`)
}

export function guestCalculate(body: {
  object_type: ObjectType
  params: ParamsMap
  seed?: number
  include_ids?: string[]
  overrides?: EconOverrides
  schema_version?: number
  collections?: State
}): Promise<CalculateResult> {
  return request<CalculateResult>('/api/guest/calculate', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function createProject(body: ProjectCreate): Promise<Project> {
  return request<Project>('/api/projects', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

// importProject creates a project from a snapshot of the browser store: the guest's work, saved to an account
// after they sign in. The server renews the record identifiers, so the same snapshot can be saved twice.
export function importProject(body: { name: string; schema_version: number; collections: State }): Promise<Project> {
  return request<Project>('/api/projects/import', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function fetchProject(id: string): Promise<Project> {
  return request<Project>(`/api/projects/${id}`)
}

export function calculateProject(
  id: string,
  body?: { seed?: number; include_ids?: string[]; overrides?: EconOverrides },
): Promise<CalculationResponse> {
  return request<CalculationResponse>(`/api/projects/${id}/calculations`, {
    method: 'POST',
    body: JSON.stringify(body ?? {}),
  })
}

// MatchView "brief" returns items without reasons and rule steps.
export type MatchView = 'full' | 'brief'

function matchQuery(view: MatchView): string {
  return view === 'brief' ? '?view=brief' : ''
}

export function guestMatch(
  body: {
    object_type: ObjectType
    params: ParamsMap
    include_ids?: string[]
    task_codes?: string[]
  },
  view: MatchView = 'full',
): Promise<MatchOutput> {
  return request<MatchOutput>(`/api/guest/match${matchQuery(view)}`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

// fetchProjectMatch ranks the catalog for the project draft and saves nothing.
export function fetchProjectMatch(id: string, view: MatchView = 'full'): Promise<MatchOutput> {
  return request<MatchOutput>(`/api/projects/${id}/match${matchQuery(view)}`)
}

export function fetchProcesses(projectId: string): Promise<{ items: Process[] }> {
  return request<{ items: Process[] }>(`/api/projects/${projectId}/processes`)
}

export function fetchVariants(projectId: string): Promise<{ items: SolutionVariant[] }> {
  return request<{ items: SolutionVariant[] }>(`/api/projects/${projectId}/variants`)
}

export function fetchCalculations(projectId: string): Promise<{ items: CalculationRunListItem[] }> {
  return request<{ items: CalculationRunListItem[] }>(`/api/projects/${projectId}/calculations`)
}

export function fetchCalculation(runId: string): Promise<CalculationResponse> {
  return request<CalculationResponse>(`/api/calculations/${runId}`)
}

export function fetchRuns(projectId: string): Promise<RunHistory> {
  return request<RunHistory>(`/api/projects/${projectId}/runs`)
}

export function fetchProjectVersion(projectId: string, versionId: string): Promise<ProjectSnapshot> {
  return request<ProjectSnapshot>(`/api/projects/${projectId}/versions/${versionId}`)
}

export function runExportPath(runId: string, kind: RunKind, format: ExportFormat): string {
  const base = kind === 'simulation' ? '/api/simulation-runs' : '/api/calculations'
  return `${base}/${runId}/export?format=${format}`
}

export function downloadRunExport(runId: string, kind: RunKind, format: ExportFormat): Promise<void> {
  return downloadFile(runExportPath(runId, kind, format), { method: 'POST', body: JSON.stringify({}) }, `run-${runId.slice(0, 8)}.${format}`)
}

export function login(body: { email: string; password: string }): Promise<AuthResponse> {
  return request<AuthResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function register(body: { email: string; password: string; invite_token?: string }): Promise<AuthResponse> {
  return request<AuthResponse>('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function verifyEmail(token: string): Promise<{ status: string }> {
  return request<{ status: string }>('/api/auth/verify-email', { method: 'POST', body: JSON.stringify({ token }) })
}

export function resendVerification(): Promise<{ status: string }> {
  return request<{ status: string }>('/api/auth/verify-email/resend', { method: 'POST' })
}

export function requestPasswordReset(email: string): Promise<{ message: string }> {
  return request<{ message: string }>('/api/auth/password-reset/request', { method: 'POST', body: JSON.stringify({ email }) })
}

export function confirmPasswordReset(token: string, password: string): Promise<{ status: string }> {
  return request<{ status: string }>('/api/auth/password-reset/confirm', {
    method: 'POST',
    body: JSON.stringify({ token, password }),
  })
}

export type InvitationCheck = {
  valid: boolean
  email?: string
  project_name?: string
  project_role?: 'editor' | 'viewer'
  reason?: string
}

export function inspectInvitation(token: string): Promise<InvitationCheck> {
  return request<InvitationCheck>('/api/auth/invitations/inspect', { method: 'POST', body: JSON.stringify({ token }) })
}

export function logout(): Promise<void> {
  return request<void>('/api/auth/logout', { method: 'POST' })
}

// Operation sync (docs/adr/0003-operations-and-sync.md). The store in web/src/store is the only caller.

export function fetchSnapshot(projectId: string): Promise<SnapshotResponse> {
  return request<SnapshotResponse>(`/api/projects/${projectId}/snapshot`)
}

export function fetchOperations(projectId: string, after: number): Promise<OperationsPage> {
  return request<OperationsPage>(`/api/projects/${projectId}/operations?after=${after}`)
}

export function postTransactions(projectId: string, body: BatchBody): Promise<BatchResponse> {
  return request<BatchResponse>(`/api/projects/${projectId}/transactions`, { method: 'POST', body: JSON.stringify(body) })
}

export type PresenceBody = {
  client_id: string
  route?: string
  selection?: { coll: string; id: string } | null
  leave?: boolean
}

// postPresence reports this tab; keepalive lets the leave report outlive the page.
export function postPresence(projectId: string, body: PresenceBody, keepalive = false): Promise<void> {
  return request<void>(`/api/projects/${projectId}/presence`, { method: 'POST', body: JSON.stringify(body), keepalive })
}

export function listProjects(): Promise<{ items: ProjectListItem[] }> {
  return request<{ items: ProjectListItem[] }>('/api/projects')
}

export type TrashedProject = { deleted_at: string | null; purge_at: string | null }

export type TrashItem = {
  id: string
  name: string
  object_type: ObjectType
  deleted_at: string | null
  purge_at: string | null
  created_at: string | null
}

export function deleteProject(id: string): Promise<TrashedProject> {
  return request<TrashedProject>(`/api/projects/${id}`, { method: 'DELETE' })
}

export function listTrash(): Promise<{ items: TrashItem[] }> {
  return request<{ items: TrashItem[] }>('/api/projects/trash')
}

export function restoreProject(id: string): Promise<Project> {
  return request<Project>(`/api/projects/${id}/restore`, { method: 'POST' })
}

export function purgeProject(id: string): Promise<void> {
  return request<void>(`/api/projects/${id}/purge`, { method: 'DELETE' })
}

export function copyProject(id: string, name?: string): Promise<Project> {
  return request<Project>(`/api/projects/${id}/copy`, {
    method: 'POST',
    body: JSON.stringify(name === undefined ? {} : { name }),
  })
}

export function downloadGuestExport(
  body: {
    object_type: ObjectType
    params: ParamsMap
    seed?: number
    include_ids?: string[]
    overrides?: EconOverrides
  },
  format: 'pdf' | 'xlsx',
): Promise<void> {
  return downloadFile(
    `/api/guest/export?format=${format}`,
    { method: 'POST', body: JSON.stringify(body) },
    `ocenka-${body.object_type}.${format}`,
  )
}

export function downloadProjectExport(id: string, format: ExportFormat, objectType: string): Promise<void> {
  return downloadFile(
    `/api/projects/${id}/export?format=${format}`,
    { method: 'POST', body: JSON.stringify({}) },
    `raschet-${objectType}.${format}`,
  )
}

// FieldValues maps catalog field codes to values in stored units; null clears a field.
export type FieldValues = Record<string, unknown>

export type FieldChange = { field: string; before: unknown; after: unknown }

// SolutionSave is what a catalog edit returns: the robot as stored and the fields that changed, for undo.
export type SolutionSave = { solution: Solution; changes: FieldChange[] }

export function adminCreateSolution(values: FieldValues): Promise<SolutionSave> {
  return request<SolutionSave>('/api/admin/solutions', { method: 'POST', body: JSON.stringify(values) })
}

export function adminPatchSolution(id: string, values: FieldValues): Promise<SolutionSave> {
  return request<SolutionSave>(`/api/admin/solutions/${id}`, { method: 'PATCH', body: JSON.stringify(values) })
}

export function adminArchiveSolution(id: string): Promise<void> {
  return request<void>(`/api/admin/solutions/${id}`, { method: 'DELETE' })
}

export function adminRestoreSolution(id: string): Promise<Solution> {
  return request<Solution>(`/api/admin/solutions/${id}/restore`, { method: 'POST' })
}

export function adminDuplicateSolution(id: string): Promise<Solution> {
  return request<Solution>(`/api/admin/solutions/${id}/duplicate`, { method: 'POST' })
}

export function adminPutSolutionImage(id: string, file: Blob): Promise<Solution> {
  return request<Solution>(`/api/admin/solutions/${id}/image`, {
    method: 'PUT',
    body: file,
    headers: { 'Content-Type': file.type || 'application/octet-stream' },
  })
}

export function adminDeleteSolutionImage(id: string): Promise<Solution> {
  return request<Solution>(`/api/admin/solutions/${id}/image`, { method: 'DELETE' })
}

export type CatalogFileFormat = 'xlsx' | 'csv'

export function downloadCatalog(format: CatalogFileFormat): Promise<void> {
  return downloadFile(`/api/admin/catalog/export?format=${format}`, { method: 'GET' }, `katalog.${format}`)
}

export function downloadCatalogTemplate(layout: 'robot' | 'catalog', format: CatalogFileFormat): Promise<void> {
  const name = layout === 'robot' ? 'shablon-resheniya' : 'shablon-kataloga'
  return downloadFile(`/api/admin/catalog/template?layout=${layout}&format=${format}`, { method: 'GET' }, `${name}.${format}`)
}

export function downloadImportErrors(id: string, format: CatalogFileFormat): Promise<void> {
  return downloadFile(`/api/admin/catalog/imports/${id}/errors?format=${format}`, { method: 'GET' }, `oshibki.${format}`)
}

export type ImportMode = 'robot' | 'catalog'

export type ImportSheet = { name: string; rows: string[][] }

export type ImportColumn = {
  index: number
  header: string
  // field is the catalog field the column fills, or null when it is not loaded.
  field: string | null
  samples: string[]
}

export type RowClass = 'new' | 'changed' | 'conflict' | 'unchanged' | 'error'

export type FieldDiff = {
  code: string
  // base is the value at the export the file came from; set when the catalog changed the field too.
  base?: unknown
  current?: unknown
  file?: unknown
  conflict?: boolean
}

export type RobotRef = { id: string; name: string }

export type ImportRow = {
  key: string
  line: number
  class: RowClass
  id?: string
  name: string
  rev?: string
  archived?: boolean
  duplicate_of?: RobotRef
  photo?: string
  fields: FieldDiff[]
  errors?: FieldError[]
}

export type ImportPlan = {
  three_way: boolean
  rows: ImportRow[]
  missing: RobotRef[]
  counts: Partial<Record<RowClass, number>>
}

export type RobotChange = { id: string; name: string; changes: FieldChange[] }

export type ImportSummary = {
  applied: { created: RobotChange[]; updated: RobotChange[]; archived: RobotRef[]; photos: { solution_id: string; name: string; url: string }[] }
  photos_total: number
  photos_done: number
  photo_errors: { solution_id: string; name: string; url: string; message: string }[]
  photos_skipped?: number
  photos_interrupted?: boolean
  applied_at: string
  unchanged: number
}

// CatalogImport is an upload session: the file, how its columns map to fields, and what applying it would do.
export type CatalogImport = {
  id: string
  file_name: string
  mode: ImportMode
  // status is draft until applied, photos while photo links are fetched, then done.
  status: 'draft' | 'photos' | 'done'
  sheets: string[]
  sheet: number
  layout: 'table' | 'form' | ''
  hint?: string
  stamp: { export_id: string; created_at: string | null; found: boolean } | null
  columns: ImportColumn[]
  mapping_error?: string
  plan?: ImportPlan
  summary?: ImportSummary
}

export function createCatalogImport(body: { file_name: string; mode: ImportMode; sheets: ImportSheet[] }): Promise<CatalogImport> {
  return request<CatalogImport>('/api/admin/catalog/imports', { method: 'POST', body: JSON.stringify(body) })
}

export function fetchCatalogImport(id: string): Promise<CatalogImport> {
  return request<CatalogImport>(`/api/admin/catalog/imports/${id}`)
}

// mapCatalogImport sets the sheet and which field each column fills, keyed by column index. Without a mapping the
// server maps the sheet's headers anew.
export function mapCatalogImport(id: string, sheet: number, mapping?: Record<string, string>): Promise<CatalogImport> {
  return request<CatalogImport>(`/api/admin/catalog/imports/${id}`, { method: 'PATCH', body: JSON.stringify({ sheet, mapping }) })
}

export type ImportApply = {
  rows: { key: string; rev: string }[]
  fields: string[]
  // picks choose the file side of a conflict per row key and field code; the catalog side is the default.
  picks: Record<string, Record<string, 'file' | 'catalog'>>
  archive: string[]
}

export function applyCatalogImport(id: string, body: ImportApply): Promise<CatalogImport> {
  return request<CatalogImport>(`/api/admin/catalog/imports/${id}/apply`, { method: 'POST', body: JSON.stringify(body) })
}

export function deleteCatalogImport(id: string): Promise<void> {
  return request<void>(`/api/admin/catalog/imports/${id}`, { method: 'DELETE' })
}

export type Invitation = {
  id: string
  email: string
  project_id: string | null
  project_name: string | null
  project_role: 'editor' | 'viewer' | null
  created_at: string | null
  expires_at: string | null
  accepted_at: string | null
  revoked_at: string | null
  invited_by: string | null
}

export function listInvitations(): Promise<{ items: Invitation[] }> {
  return request<{ items: Invitation[] }>('/api/admin/invitations?limit=200')
}

export function createInvitation(body: { email: string; project_id?: string; project_role?: 'editor' | 'viewer' }): Promise<Invitation> {
  return request<Invitation>('/api/admin/invitations', { method: 'POST', body: JSON.stringify(body) })
}

export function revokeInvitation(id: string): Promise<void> {
  return request<void>(`/api/admin/invitations/${id}`, { method: 'DELETE' })
}

export type AuditEvent = {
  id: number
  at: string | null
  // actor_email is masked by the server (i***@mail.ru); null for the system.
  actor_email: string | null
  action: string
  target_type: string
  target_id: string | null
  // target_name is the current name of the project or robot the event is about; null once it is gone.
  target_name: string | null
  meta: Record<string, unknown>
}

export type AuditQuery = { actions?: string[]; target_type?: string; before?: number; limit?: number }

export function fetchAuditEvents(q: AuditQuery): Promise<{ items: AuditEvent[] }> {
  const p = new URLSearchParams()
  if (q.actions && q.actions.length > 0) p.set('action', q.actions.join(','))
  if (q.target_type) p.set('target_type', q.target_type)
  if (q.before != null) p.set('before', String(q.before))
  if (q.limit != null) p.set('limit', String(q.limit))
  const qs = p.toString()
  return request<{ items: AuditEvent[] }>(`/api/admin/audit${qs ? `?${qs}` : ''}`)
}

export type MapXY = {
  x: number
  y: number
}

export type MapSegment = {
  x1: number
  y1: number
  x2: number
  y2: number
  length_m: number
}

export type MapPolygon = {
  id: string
  kind: string
  name?: string
  ring: MapXY[]
}

export type MapPointKind = 'task' | 'dock' | 'charger' | 'gate' | 'other'

export type MapPoint = {
  id: string
  kind: MapPointKind
  name?: string
  x: number
  y: number
  process_code?: string
}

export type MapResourceKind = 'dock' | 'narrow_aisle' | 'charger'

export type MapResource = {
  id: string
  kind: MapResourceKind
  name?: string
  capacity: number
  point_id?: string
  point_ids?: string[]
  edge_ids?: string[]
}

export type MapEdge = {
  id: string
  from: string
  to: string
  bidirectional?: boolean
  width_m?: number
}

export type MapFlow = {
  id?: string
  process_code: string
  pickup_point_ids: string[]
  drop_point_ids: string[]
}

export type MapSourceKind = 'png' | 'jpeg' | 'pdf' | 'none'

export type MapDocument = {
  schema_version: 'map-v1'
  profile: 'indoor' | 'airspace' | 'field'
  units: 'm'
  page?: { width_px: number; height_px: number; source_kind: MapSourceKind }
  calibration: { meters_per_px: number; segment?: MapSegment; check?: MapSegment }
  layers: {
    zones: MapPolygon[]
    obstacles: MapPolygon[]
    points: MapPoint[]
    resources: MapResource[]
    edges: MapEdge[]
    flows?: MapFlow[]
  }
  errors?: string[]
  warnings?: string[]
}

export type MapIssue = {
  level: 'error' | 'warning'
  code: string
  message: string
  ref?: string
}

export type MapEdgeCheck = {
  id: string
  length_m: number
  width_m: number
  declared_width_m?: number
  measured_width_m?: number
  single_lane: boolean
  blocked_for?: string[]
}

export type MapClassCheck = {
  code: string
  label: string
  required_width_m: number
  two_lane_width_m: number
  default?: boolean
}

export type ScenePolygon = {
  id: string
  kind: string
  name?: string
  ring: MapXY[]
}

export type SimScene = {
  width_m: number
  height_m: number
  zones: ScenePolygon[]
  obstacles: ScenePolygon[]
}

export type MapCheck = {
  issues: MapIssue[]
  edges: MapEdgeCheck[]
  classes: MapClassCheck[]
  scene?: SimScene
}

export type SimMode = 'stochastic' | 'deterministic'
export type SimPolicy = 'fifo' | 'nearest' | 'sla_priority'

export type SimWindow = {
  start_h: number
  end_h: number
}

export type SimulationConfig = {
  mode?: SimMode
  replications?: number
  seed?: number
  policy?: SimPolicy
  horizon_h?: number
  schedule?: SimWindow[]
  demand_profile?: number[]
  sla_target_pct?: number
}

export type SimulationRequest = SimulationConfig & {
  variant_id?: string
  line_key?: string
}

export type SimJobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled'

export type SimulationJob = {
  id: string
  project_id: string
  run_id: string | null
  status: SimJobStatus
  progress_pct: number
  attempt: number
  max_attempts: number
  error_text: string | null
  replications_total: number
  replications_done: number
  config: { variant_id?: string; sim?: SimulationConfig }
  created_at: string | null
  updated_at: string | null
  started_at: string | null
  finished_at: string | null
  canceled_at: string | null
}

export type SimulationPreview = {
  variant_id: string
  variant_name: string
  map_source: 'project' | 'template'
  confidence_level: string
  replications: number
  expected_jobs: Record<string, number>
  warnings: string[]
  map_issues: MapIssue[]
}

export type SimulationCreateResult = {
  job: SimulationJob
  run_id: string
  version_no: number
  input_hash: string
  preview: SimulationPreview
}

export type SimStat = {
  median: number
  min: number
  max: number
  p10: number
  p90: number
}

export type SimKPI = {
  arrived: SimStat
  completed: SimStat
  violated: SimStat
  violation_rate: SimStat
  throughput_per_h: SimStat
  wait_mean_s: SimStat
  wait_p95_s: SimStat
  cycle_mean_s: SimStat
  cycle_p95_s: SimStat
  queue_mean: SimStat
  queue_max: SimStat
  fleet_utilization: SimStat
  charging_share: SimStat
  idle_share: SimStat
  distance_km: SimStat
  battery_depleted: SimStat
  dispatch_wait_s: SimStat
}

export type SimProcessResult = {
  code: string
  name: string
  task_type: string
  covered: boolean
  priority: number
  units_per_job: number
  expected_jobs: number
  max_wait_s: number
  max_cycle_s: number
  arrived: SimStat
  completed: SimStat
  violated: SimStat
  violation_rate: SimStat
  throughput_per_h: SimStat
  wait_mean_s: SimStat
  wait_p95_s: SimStat
  cycle_mean_s: SimStat
  cycle_p95_s: SimStat
  queue_mean: SimStat
  queue_max: SimStat
}

export type SimResourceResult = {
  id: string
  kind: MapResourceKind
  name: string
  capacity: number
  auto?: boolean
  utilization: SimStat
  queue_mean: SimStat
  queue_max: SimStat
  wait_mean_s: SimStat
  wait_max_s: SimStat
  wait_total_s: SimStat
}

export type SimFleetResult = {
  key: string
  solution_id: string
  name: string
  profile: 'amr' | 'pallet'
  profile_label: string
  quantity: number
  serves?: string[]
  processes: string[]
  speed_mps: number
  width_m: number
  endurance_h: number
  charge_min: number
  assumed?: string[]
}

export type SimRobotMetrics = {
  id: string
  jobs: number
  distance_km: number
  utilization: number
  charging_share: number
  idle_share: number
  battery_end: number
  depleted: number
  states_s: Record<string, number>
}

export type SimBottleneck = {
  kind: 'fleet' | MapResourceKind
  id: string
  name: string
  primary: boolean
  saturated: boolean
  wait_s: number
  fleet_share: number
  utilization: number
  queue_max: number
  text: string
}

export type SimulationResult = {
  sim_version: string
  config: SimulationConfig
  replications: number
  seeds: number[]
  horizon_s: number
  windows: SimWindow[]
  demand_profile: number[]
  active_hours_per_day: number
  sla_target_pct: number
  sla_ok: boolean
  verdict: 'pass' | 'fail'
  verdict_text: string
  battery_modeled: boolean
  kpi: SimKPI
  processes: SimProcessResult[]
  resources: SimResourceResult[]
  fleet: SimFleetResult[]
  robots: SimRobotMetrics[]
  bottlenecks: SimBottleneck[]
  representative: number
  representative_seed: number
  warnings: string[]
  assumptions: string[]
}

export type SimulationSummary = {
  schema_version: string
  run_id: string
  project_id: string
  version_no: number
  input_hash: string
  catalog_content_sha256?: string
  variant_id: string
  variant_name: string
  // variant_hash names the inputs of the variant the run read; a run is current while the variant still hashes so.
  variant_hash?: string
  map_source: 'project' | 'template'
  map_warnings: string[]
  confidence_level: string
  finished_at: string
  duration_ms: number
  result: SimulationResult
  econ_check?: { expected_jobs: number; completed: number; coverage: number; flag: boolean; text: string }
  fleet_search?: {
    line_key: string
    rows: { quantity: number; coverage: number; sla_ok: boolean }[]
    best: number
    stopped?: string
  }
  snapshot: { match_version: string; econ_version: string; sim_version: string }
}

export type SimulationRunInfo = {
  id: string
  project_id: string
  project_version_id: string
  input_hash: string
  match_version: string
  econ_version: string
  sim_version: string
  seed: number
  status: SimJobStatus
  created_at: string | null
  version_no: number
  confidence_level: ConfidenceLevel
}

export type SimulationRunResponse = {
  run: SimulationRunInfo
  job: SimulationJob | null
  stale_vs_draft: boolean
  summary: SimulationSummary | null
  replications: { idx: number; seed: number; status: string; metrics: Record<string, unknown> }[]
  artifact: {
    kind: string
    encoding: string
    size_bytes: number
    raw_bytes: number
    event_count: number
    pinned: boolean
    expires_at: string | null
    created_at: string | null
  } | null
}

export type SimulationBrief = {
  verdict: 'pass' | 'fail' | null
  verdict_text: string | null
  kpi: SimKPI | null
  fleet: SimFleetResult[] | null
  variant_name: string | null
  map_source: 'project' | 'template' | null
}

export type SimulationListItem = SimulationJob & {
  input_hash: string
  seed: number
  sim_version: string
  project_version_id: string
  stale_vs_draft: boolean
  brief: SimulationBrief
}

export type SimEventType =
  | 'job_arrival'
  | 'dispatch'
  | 'route_start'
  | 'route_end'
  | 'resource_acquire'
  | 'resource_release'
  | 'load'
  | 'unload'
  | 'charge_start'
  | 'charge_end'
  | 'sla_reached'
  | 'sla_violated'

export type SimEvent = {
  seq: number
  t_s: number
  type: SimEventType
  job_id?: string
  robot_id?: string
  process_code?: string
  resource_id?: string
  from_id?: string
  to_id?: string
  priority?: number
  payload?: {
    path?: string[]
    dist_m?: number
    dur_s?: number
    loaded?: boolean
    battery?: number
    wait_s?: number
    cycle_s?: number
    kind?: string
    queue?: number
  }
}

export type SimLogNode = {
  id: string
  kind: MapPointKind
  name?: string
  x: number
  y: number
}

export type SimLogEdge = {
  id: string
  from: string
  to: string
  length_m: number
  width_m: number
  two_way: boolean
}

export type SimLogResource = {
  id: string
  kind: MapResourceKind
  name: string
  capacity: number
  auto?: boolean
  node_ids?: string[]
  edge_ids?: string[]
}

export type SimLogRobot = {
  id: string
  name: string
  profile: 'amr' | 'pallet'
  width_m: number
  length_m: number
  fleet_key: string
  start_id: string
  battery: number
}

export type SimCheckpoint = {
  t_s: number
  queue: number
  busy: number
  idle: number
  charging: number
  completed: number
  violated: number
  processes: number[]
  resources: { id: string; in_use: number; queue: number }[]
}

export type SimulationLog = {
  schema_version: string
  event_schema: string
  sim_version: string
  replication: number
  seed: number
  horizon_s: number
  scene: SimScene
  nodes: SimLogNode[]
  edges: SimLogEdge[]
  resources: SimLogResource[]
  robots: SimLogRobot[]
  processes: { code: string; name: string; covered: boolean; priority: number }[]
  events: SimEvent[]
  checkpoints: SimCheckpoint[]
  truncated: boolean
}

export function validateProjectMap(projectId: string, doc: MapDocument, signal?: AbortSignal): Promise<MapCheck> {
  return request<MapCheck>(`/api/projects/${projectId}/map/validate`, {
    method: 'POST',
    body: JSON.stringify(doc),
    signal,
  })
}

export function fetchMapTemplate(projectId: string): Promise<MapDocument> {
  return request<MapDocument>(`/api/projects/${projectId}/map/template`)
}

export function createSimulation(projectId: string, body: SimulationRequest): Promise<SimulationCreateResult> {
  return request<SimulationCreateResult>(`/api/projects/${projectId}/simulations`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function fetchSimulations(projectId: string): Promise<{ items: SimulationListItem[]; input_hash: string }> {
  return request(`/api/projects/${projectId}/simulations`)
}

export function fetchSimChecks(projectId: string): Promise<{ items: EconSimCheck[] }> {
  return request(`/api/projects/${projectId}/sim-checks`)
}

export function fetchSimulationJob(jobId: string): Promise<SimulationJob> {
  return request<SimulationJob>(`/api/simulation-jobs/${jobId}`)
}

export function cancelSimulationJob(jobId: string): Promise<SimulationJob> {
  return request<SimulationJob>(`/api/simulation-jobs/${jobId}`, { method: 'DELETE' })
}

export function retrySimulationJob(jobId: string): Promise<SimulationJob> {
  return request<SimulationJob>(`/api/simulation-jobs/${jobId}/retry`, { method: 'POST' })
}

export function fetchSimulationRun(runId: string): Promise<SimulationRunResponse> {
  return request<SimulationRunResponse>(`/api/simulation-runs/${runId}`)
}

export function fetchSimulationEvents(runId: string): Promise<SimulationLog> {
  return request<SimulationLog>(`/api/simulation-runs/${runId}/events`)
}

export function pinSimulationRun(runId: string, pinned: boolean): Promise<{ pinned: boolean; expires_at: string | null }> {
  return request(`/api/simulation-runs/${runId}/pin`, {
    method: 'PUT',
    body: JSON.stringify({ pinned }),
  })
}
