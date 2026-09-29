-- name: CountSolutions :one
SELECT count(*)::bigint FROM solutions;

-- name: ListSolutions :many
SELECT
    s.id,
    s.name,
    s.vendor,
    s.kind,
    s.subtype,
    s.status,
    s.industry,
    s.scenario,
    s.price_rub,
    s.source_url,
    sp.payload_kg,
    sp.mass_kg,
    sp.length_mm,
    sp.width_mm,
    sp.height_mm,
    sp.speed_mps,
    sp.endurance_h,
    sp.charge_min,
    sp.nav_type,
    sp.pos_accuracy_mm,
    sp.min_aisle_mm,
    sp.turn_radius_mm,
    sp.temp_min_c,
    sp.temp_max_c,
    sp.lifetime_years,
    sp.service_pct_year,
    sp.confidence,
    sp.sourced_at,
    s.raw,
    s.family,
    s.archived_at,
    s.field_sources,
    img.sha256 AS image_sha
FROM solutions s
LEFT JOIN solution_specs sp ON sp.solution_id = s.id
LEFT JOIN solution_images img ON img.solution_id = s.id
ORDER BY s.name, s.id
LIMIT $1;

-- name: ListSolutionsByIDs :many
SELECT
    s.id,
    s.name,
    s.vendor,
    s.kind,
    s.subtype,
    s.price_rub,
    s.source_url,
    sp.payload_kg,
    sp.width_mm,
    sp.length_mm,
    sp.min_aisle_mm,
    sp.speed_mps,
    sp.endurance_h,
    sp.charge_min,
    sp.lifetime_years,
    sp.service_pct_year,
    sp.confidence,
    sp.sourced_at
FROM solutions s
LEFT JOIN solution_specs sp ON sp.solution_id = s.id
WHERE s.id = ANY(sqlc.arg(solution_ids)::uuid[])
ORDER BY s.name, s.id;

-- name: GetSolution :one
-- NOTE: the same columns as ListSolutions, so a row converts to ListSolutionsRow.
SELECT
    s.id,
    s.name,
    s.vendor,
    s.kind,
    s.subtype,
    s.status,
    s.industry,
    s.scenario,
    s.price_rub,
    s.source_url,
    sp.payload_kg,
    sp.mass_kg,
    sp.length_mm,
    sp.width_mm,
    sp.height_mm,
    sp.speed_mps,
    sp.endurance_h,
    sp.charge_min,
    sp.nav_type,
    sp.pos_accuracy_mm,
    sp.min_aisle_mm,
    sp.turn_radius_mm,
    sp.temp_min_c,
    sp.temp_max_c,
    sp.lifetime_years,
    sp.service_pct_year,
    sp.confidence,
    sp.sourced_at,
    s.raw,
    s.family,
    s.archived_at,
    s.field_sources,
    img.sha256 AS image_sha
FROM solutions s
LEFT JOIN solution_specs sp ON sp.solution_id = s.id
LEFT JOIN solution_images img ON img.solution_id = s.id
WHERE s.id = $1;

-- name: ListEditedSolutionIDs :many
SELECT id FROM solutions WHERE edited_at IS NOT NULL;

-- name: LockSolution :one
SELECT id FROM solutions WHERE id = $1 FOR UPDATE;

-- name: InsertSolution :execrows
INSERT INTO solutions (
    id, name, vendor, kind, subtype, status, industry, scenario, price_rub, source_url, raw, family
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO NOTHING;

-- name: InsertSolutionSpecs :exec
INSERT INTO solution_specs (solution_id)
VALUES ($1)
ON CONFLICT (solution_id) DO NOTHING;

-- name: FillSolutionUses :exec
UPDATE solutions
SET raw = jsonb_set(COALESCE(raw, '{}'::jsonb), '{uses}', $2, true)
WHERE id = $1 AND jsonb_typeof(raw->'uses') IS DISTINCT FROM 'array';

-- name: CutSolutionUses :exec
UPDATE solutions
SET raw = jsonb_set(jsonb_set(COALESCE(raw, '{}'::jsonb), '{uses}', sqlc.arg(uses)::jsonb, true), '{source_id}', to_jsonb(sqlc.arg(source_id)::text), true)
WHERE id = sqlc.arg(id) AND edited_at IS NULL AND raw->>'source_id' IS NULL;

-- name: FillFieldSources :exec
UPDATE solutions SET field_sources = sqlc.arg(sources)::jsonb || field_sources WHERE id = sqlc.arg(id);

-- name: DropFieldSources :exec
UPDATE solutions SET field_sources = field_sources - sqlc.arg(codes)::text[] WHERE id = sqlc.arg(id);

-- name: FillSolutionObjectTypes :exec
UPDATE solutions
SET raw = jsonb_set(COALESCE(raw, '{}'::jsonb), '{object_types}', $2, true)
WHERE id = $1
    AND (jsonb_typeof(raw->'object_types') IS DISTINCT FROM 'array' OR jsonb_array_length(raw->'object_types') = 0);

-- name: FillSolutionSourceURL :exec
UPDATE solutions SET source_url = $2 WHERE id = $1 AND source_url IS NULL;

-- name: FillSolutionPrice :exec
UPDATE solutions SET price_rub = $2 WHERE id = $1 AND price_rub IS NULL;

-- name: FillSolutionText :exec
UPDATE solutions
SET subtype = COALESCE(subtype, sqlc.narg(subtype)::text),
    family = COALESCE(family, sqlc.narg(family)::text),
    raw = CASE
        WHEN sqlc.narg(description)::text IS NOT NULL AND btrim(COALESCE(raw->>'описание', '')) = ''
            THEN jsonb_set(COALESCE(raw, '{}'::jsonb), '{описание}', to_jsonb(sqlc.narg(description)::text), true)
        ELSE raw
    END
WHERE id = sqlc.arg(id);

-- name: UpsertSolutionSpecs :exec
INSERT INTO solution_specs (
    solution_id,
    payload_kg,
    mass_kg,
    length_mm,
    width_mm,
    height_mm,
    speed_mps,
    endurance_h,
    charge_min,
    nav_type,
    pos_accuracy_mm,
    min_aisle_mm,
    temp_min_c,
    temp_max_c,
    lifetime_years,
    service_pct_year,
    confidence,
    sourced_at,
    turn_radius_mm
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)
ON CONFLICT (solution_id) DO UPDATE SET
    payload_kg = EXCLUDED.payload_kg,
    mass_kg = EXCLUDED.mass_kg,
    length_mm = EXCLUDED.length_mm,
    width_mm = EXCLUDED.width_mm,
    height_mm = EXCLUDED.height_mm,
    speed_mps = EXCLUDED.speed_mps,
    endurance_h = EXCLUDED.endurance_h,
    charge_min = EXCLUDED.charge_min,
    nav_type = EXCLUDED.nav_type,
    pos_accuracy_mm = EXCLUDED.pos_accuracy_mm,
    min_aisle_mm = EXCLUDED.min_aisle_mm,
    temp_min_c = EXCLUDED.temp_min_c,
    temp_max_c = EXCLUDED.temp_max_c,
    lifetime_years = EXCLUDED.lifetime_years,
    service_pct_year = EXCLUDED.service_pct_year,
    confidence = EXCLUDED.confidence,
    sourced_at = EXCLUDED.sourced_at,
    turn_radius_mm = EXCLUDED.turn_radius_mm;

-- name: FillSolutionSpecs :exec
INSERT INTO solution_specs (
    solution_id,
    payload_kg,
    mass_kg,
    length_mm,
    width_mm,
    height_mm,
    speed_mps,
    endurance_h,
    charge_min,
    nav_type,
    pos_accuracy_mm,
    min_aisle_mm,
    temp_min_c,
    temp_max_c,
    lifetime_years,
    service_pct_year,
    confidence,
    sourced_at,
    turn_radius_mm
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)
ON CONFLICT (solution_id) DO UPDATE SET
    payload_kg = COALESCE(solution_specs.payload_kg, EXCLUDED.payload_kg),
    mass_kg = COALESCE(solution_specs.mass_kg, EXCLUDED.mass_kg),
    length_mm = COALESCE(solution_specs.length_mm, EXCLUDED.length_mm),
    width_mm = COALESCE(solution_specs.width_mm, EXCLUDED.width_mm),
    height_mm = COALESCE(solution_specs.height_mm, EXCLUDED.height_mm),
    speed_mps = COALESCE(solution_specs.speed_mps, EXCLUDED.speed_mps),
    endurance_h = COALESCE(solution_specs.endurance_h, EXCLUDED.endurance_h),
    charge_min = COALESCE(solution_specs.charge_min, EXCLUDED.charge_min),
    nav_type = COALESCE(solution_specs.nav_type, EXCLUDED.nav_type),
    pos_accuracy_mm = COALESCE(solution_specs.pos_accuracy_mm, EXCLUDED.pos_accuracy_mm),
    min_aisle_mm = COALESCE(solution_specs.min_aisle_mm, EXCLUDED.min_aisle_mm),
    temp_min_c = COALESCE(solution_specs.temp_min_c, EXCLUDED.temp_min_c),
    temp_max_c = COALESCE(solution_specs.temp_max_c, EXCLUDED.temp_max_c),
    lifetime_years = COALESCE(solution_specs.lifetime_years, EXCLUDED.lifetime_years),
    service_pct_year = COALESCE(solution_specs.service_pct_year, EXCLUDED.service_pct_year),
    confidence = COALESCE(solution_specs.confidence, EXCLUDED.confidence),
    sourced_at = COALESCE(solution_specs.sourced_at, EXCLUDED.sourced_at),
    turn_radius_mm = COALESCE(solution_specs.turn_radius_mm, EXCLUDED.turn_radius_mm);

-- name: CreateProject :one
INSERT INTO projects (user_id, name, object_type, params, model_version, input_hash)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, name, object_type, params, results, model_version, calc_seed, created_at,
    status, updated_at, deleted_at, current_run_id, input_hash;

-- name: GetProject :one
SELECT id, user_id, name, object_type, params, results, model_version, calc_seed, created_at,
    status, updated_at, deleted_at, current_run_id, input_hash
FROM projects
WHERE id = $1;

-- name: ChangeProjectObjectType :one
UPDATE projects
SET name = $2, object_type = $3, params = $4, results = NULL, current_run_id = NULL, updated_at = now()
WHERE id = $1
RETURNING id, user_id, name, object_type, params, results, model_version, calc_seed, created_at,
    status, updated_at, deleted_at, current_run_id, input_hash;

-- name: SetProjectCalculation :one
UPDATE projects
SET results = $2, model_version = $3, calc_seed = $4, current_run_id = $5, input_hash = $6, updated_at = now()
WHERE id = $1
RETURNING id, user_id, name, object_type, params, results, model_version, calc_seed, created_at,
    status, updated_at, deleted_at, current_run_id, input_hash;

-- name: SetProjectInputHash :exec
UPDATE projects
SET input_hash = $2, updated_at = now()
WHERE id = $1 AND input_hash IS DISTINCT FROM $2;

-- name: ListProjectsByUser :many
SELECT p.id, p.name, p.object_type, p.model_version, p.calc_seed, p.created_at,
    (p.current_run_id IS NOT NULL OR p.results IS NOT NULL)::boolean AS has_results,
    CASE
        WHEN p.current_run_id IS NULL THEN false
        ELSE (COALESCE(r.draft_hash, r.input_hash) IS DISTINCT FROM p.input_hash)
    END::boolean AS stale,
    p.current_run_id,
    p.input_hash,
    (CASE WHEN p.user_id = sqlc.arg(user_id)::uuid THEN 'owner' ELSE m.role END)::text AS access
FROM projects p
LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = sqlc.arg(user_id)::uuid
LEFT JOIN calculation_runs r ON r.id = p.current_run_id
WHERE (p.user_id = sqlc.arg(user_id)::uuid OR m.user_id IS NOT NULL) AND p.deleted_at IS NULL
ORDER BY p.created_at DESC
LIMIT sqlc.arg(max_rows);

-- name: CopyProject :one
INSERT INTO projects (user_id, name, object_type, params, model_version)
SELECT sqlc.arg(owner_id)::uuid, sqlc.arg(name), p.object_type, p.params, p.model_version
FROM projects p
WHERE p.id = sqlc.arg(id) AND p.deleted_at IS NULL
RETURNING id, user_id, name, object_type, params, results, model_version, calc_seed, created_at,
    status, updated_at, deleted_at, current_run_id, input_hash;

-- name: GetProjectAccess :one
SELECT p.id, p.user_id, p.name, p.object_type, p.params, p.results, p.model_version, p.calc_seed, p.created_at,
    p.status, p.updated_at, p.deleted_at, p.current_run_id, p.input_hash,
    (CASE WHEN p.user_id = sqlc.arg(user_id)::uuid THEN 'owner' ELSE m.role END)::text AS role
FROM projects p
LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = sqlc.arg(user_id)::uuid
WHERE p.id = sqlc.arg(project_id)
    AND (p.user_id = sqlc.arg(user_id)::uuid OR m.user_id IS NOT NULL);

-- name: GetRunProjectID :one
SELECT project_id
FROM calculation_runs
WHERE id = $1;

-- name: GetJobProjectID :one
SELECT project_id
FROM simulation_jobs
WHERE id = $1;

-- name: ListProjectMembers :many
SELECT m.user_id, u.email, m.role, m.created_at
FROM project_members m
JOIN users u ON u.id = m.user_id
WHERE m.project_id = $1
ORDER BY m.created_at, u.email;

-- name: AddProjectMember :execrows
INSERT INTO project_members (project_id, user_id, role, added_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (project_id, user_id) DO NOTHING;

-- name: SetProjectMemberRole :execrows
UPDATE project_members
SET role = $3
WHERE project_id = $1 AND user_id = $2;

-- name: RemoveProjectMember :execrows
DELETE FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, role, created_at, email_verified_at, disabled_at
FROM users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, role, created_at, email_verified_at, disabled_at
FROM users
WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, role, email_verified_at)
VALUES ($1, $2, $3, sqlc.narg(email_verified_at)::timestamptz)
RETURNING id, email, password_hash, role, created_at, email_verified_at, disabled_at;

-- name: CreateSolution :exec
INSERT INTO solutions (
    id, name, vendor, kind, subtype, status, industry, scenario, price_rub, source_url, raw, family, edited_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now()
);

-- name: SaveSolution :exec
UPDATE solutions
SET name = $2,
    vendor = $3,
    kind = $4,
    subtype = $5,
    status = $6,
    industry = $7,
    scenario = $8,
    price_rub = $9,
    source_url = $10,
    raw = $11,
    family = $12,
    edited_at = now()
WHERE id = $1;

-- name: ArchiveSolution :execrows
UPDATE solutions SET archived_at = now() WHERE id = $1 AND archived_at IS NULL;

-- name: RestoreSolution :execrows
UPDATE solutions SET archived_at = NULL WHERE id = $1 AND archived_at IS NOT NULL;

-- name: UpsertSolutionImage :exec
INSERT INTO solution_images (solution_id, content_type, bytes, sha256, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (solution_id) DO UPDATE SET
    content_type = EXCLUDED.content_type,
    bytes = EXCLUDED.bytes,
    sha256 = EXCLUDED.sha256,
    updated_at = now();

-- name: GetSolutionImage :one
SELECT solution_id, content_type, bytes, sha256, updated_at
FROM solution_images
WHERE solution_id = $1;

-- name: DeleteSolutionImage :execrows
DELETE FROM solution_images WHERE solution_id = $1;

-- name: CopySolutionImage :exec
INSERT INTO solution_images (solution_id, content_type, bytes, sha256, updated_at)
SELECT sqlc.arg(to_id)::uuid, content_type, bytes, sha256, now()
FROM solution_images
WHERE solution_id = sqlc.arg(from_id)::uuid;

-- name: InsertCatalogExport :one
INSERT INTO catalog_exports (created_by, snapshot)
VALUES ($1, $2)
RETURNING id, created_at;

-- name: GetCatalogExport :one
SELECT id, created_by, created_at, snapshot
FROM catalog_exports
WHERE id = $1;

-- name: InsertCatalogImport :one
INSERT INTO catalog_imports (created_by, file_name, mode, export_id, grid, mapping)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: GetCatalogImport :one
SELECT id, created_by, created_at, file_name, mode, export_id, grid, mapping, status, summary
FROM catalog_imports
WHERE id = $1;

-- name: LockCatalogImport :one
SELECT status FROM catalog_imports WHERE id = $1 FOR UPDATE;

-- name: SetCatalogImportMapping :exec
UPDATE catalog_imports SET mapping = $2, export_id = $3 WHERE id = $1;

-- name: SetCatalogImportResult :exec
UPDATE catalog_imports SET status = $2, summary = $3 WHERE id = $1;

-- name: DeleteCatalogImport :execrows
DELETE FROM catalog_imports WHERE id = $1;

-- name: DeleteStaleCatalogImportDrafts :execrows
DELETE FROM catalog_imports
WHERE status = 'draft' AND created_at < sqlc.arg(before)::timestamptz;

-- name: FinishInterruptedCatalogImports :execrows
UPDATE catalog_imports
SET status = 'done', summary = jsonb_set(summary, '{photos_interrupted}', 'true'::jsonb, true)
WHERE status = 'photos';

-- name: UpsertProjectDraft :one
INSERT INTO project_drafts (project_id, document, input_hash, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (project_id) DO UPDATE SET
    document = EXCLUDED.document,
    input_hash = EXCLUDED.input_hash,
    updated_at = now()
RETURNING id, project_id, document, input_hash, updated_at;

-- name: GetProjectDraft :one
SELECT id, project_id, document, input_hash, updated_at
FROM project_drafts
WHERE project_id = $1;

-- name: ListProjectProcesses :many
SELECT id, project_id, code, name, task_type, is_baseline, demand, sla, point_ids, durations, baseline_staff, sort_order, order_key
FROM project_processes
WHERE project_id = $1
ORDER BY sort_order, code;

-- name: InsertProjectProcess :one
INSERT INTO project_processes (
    id, project_id, code, name, task_type, is_baseline, demand, sla, point_ids, durations, baseline_staff, sort_order, order_key
) VALUES (
    COALESCE(sqlc.narg(id)::uuid, gen_random_uuid()), sqlc.arg(project_id), sqlc.arg(code), sqlc.arg(name),
    sqlc.arg(task_type), sqlc.arg(is_baseline), sqlc.arg(demand), sqlc.arg(sla), sqlc.arg(point_ids),
    sqlc.arg(durations), sqlc.arg(baseline_staff), sqlc.arg(sort_order), sqlc.arg(order_key)
)
RETURNING id, project_id, code, name, task_type, is_baseline, demand, sla, point_ids, durations, baseline_staff, sort_order, order_key;

-- name: DeleteProjectProcesses :exec
DELETE FROM project_processes
WHERE project_id = $1;

-- name: ListSolutionVariants :many
SELECT id, project_id, name, status, notes, sort_order, created_at, updated_at, order_key
FROM solution_variants
WHERE project_id = $1
ORDER BY sort_order, created_at;

-- name: InsertSolutionVariant :one
INSERT INTO solution_variants (id, project_id, name, status, notes, sort_order, order_key)
VALUES (
    COALESCE(sqlc.narg(id)::uuid, gen_random_uuid()), sqlc.arg(project_id), sqlc.arg(name), sqlc.arg(status),
    sqlc.arg(notes), sqlc.arg(sort_order), sqlc.arg(order_key)
)
RETURNING id, project_id, name, status, notes, sort_order, created_at, updated_at, order_key;

-- name: DeleteSolutionVariants :exec
DELETE FROM solution_variants
WHERE project_id = $1;

-- name: ListVariantFleetItems :many
SELECT id, variant_id, solution_id, quantity, task_codes, price_override_rub, price_override_reason, sort_order, order_key
FROM variant_fleet_items
WHERE variant_id = $1
ORDER BY sort_order, id;

-- name: InsertVariantFleetItem :one
INSERT INTO variant_fleet_items (
    id, variant_id, solution_id, quantity, task_codes, price_override_rub, price_override_reason, sort_order, order_key
) VALUES (
    COALESCE(sqlc.narg(id)::uuid, gen_random_uuid()), sqlc.arg(variant_id), sqlc.narg(solution_id), sqlc.arg(quantity),
    sqlc.arg(task_codes), sqlc.arg(price_override_rub), sqlc.arg(price_override_reason), sqlc.arg(sort_order), sqlc.arg(order_key)
)
RETURNING id, variant_id, solution_id, quantity, task_codes, price_override_rub, price_override_reason, sort_order, order_key;

-- name: DeleteVariantFleetItems :exec
DELETE FROM variant_fleet_items
WHERE variant_id = $1;

-- name: ListFinancingScenarios :many
SELECT id, variant_id, kind, tariff, assumptions
FROM financing_scenarios
WHERE variant_id = $1
ORDER BY kind, tariff;

-- name: InsertFinancingScenario :one
INSERT INTO financing_scenarios (id, variant_id, kind, tariff, assumptions)
VALUES (
    COALESCE(sqlc.narg(id)::uuid, gen_random_uuid()), sqlc.arg(variant_id), sqlc.arg(kind), sqlc.narg(tariff),
    sqlc.arg(assumptions)
)
RETURNING id, variant_id, kind, tariff, assumptions;

-- name: DeleteFinancingScenarios :exec
DELETE FROM financing_scenarios
WHERE variant_id = $1;

-- name: NextProjectVersionNo :one
SELECT COALESCE(max(version_no), 0)::int + 1
FROM project_versions
WHERE project_id = $1;

-- name: LockProjectForVersion :one
SELECT id
FROM projects
WHERE id = $1
FOR UPDATE;

-- name: InsertProjectVersion :one
INSERT INTO project_versions (project_id, version_no, snapshot, input_hash)
VALUES ($1, $2, $3, $4)
RETURNING id, project_id, version_no, snapshot, input_hash, created_at;

-- name: GetProjectVersion :one
SELECT id, project_id, version_no, snapshot, input_hash, created_at
FROM project_versions
WHERE id = $1 AND project_id = $2;

-- name: ListProjectVersions :many
SELECT id, project_id, version_no, input_hash, created_at
FROM project_versions
WHERE project_id = $1
ORDER BY version_no DESC;

-- name: InsertCalculationRun :one
INSERT INTO calculation_runs (
    project_id, project_version_id, input_hash, draft_hash, match_version, econ_version, sim_version, seed, status,
    confidence_level, draft_seq
) VALUES (
    $1, $2, $3, $3, $4, $5, $6, $7, $8, $9, sqlc.narg(draft_seq)
)
RETURNING id, project_id, project_version_id, input_hash, match_version, econ_version, sim_version, seed, status, created_at;

-- name: ListRunsWithoutDraftHash :many
SELECT r.id, r.kind, r.input_hash, r.match_version, r.econ_version, r.sim_version, v.snapshot
FROM calculation_runs r
JOIN project_versions v ON v.id = r.project_version_id
WHERE r.project_id = $1 AND r.draft_hash IS NULL;

-- name: SetRunDraftHash :exec
UPDATE calculation_runs
SET draft_hash = sqlc.arg(draft_hash)::text
WHERE id = sqlc.arg(id);

-- name: InsertCalculationResult :one
INSERT INTO calculation_results (run_id, summary)
VALUES ($1, $2)
RETURNING run_id, summary, created_at;

-- name: GetCalculationRun :one
SELECT r.id, r.project_id, r.project_version_id, r.input_hash,
    COALESCE(r.draft_hash, r.input_hash)::text AS draft_hash, r.match_version, r.econ_version,
    r.sim_version, r.seed, r.status, r.created_at, c.summary, r.confidence_level, v.version_no
FROM calculation_runs r
JOIN calculation_results c ON c.run_id = r.id
JOIN project_versions v ON v.id = r.project_version_id
WHERE r.id = $1 AND r.kind = 'calculation';

-- name: ListCalculationRuns :many
SELECT r.id, r.project_id, r.project_version_id, r.input_hash,
    COALESCE(r.draft_hash, r.input_hash)::text AS draft_hash, r.match_version, r.econ_version,
    r.sim_version, r.seed, r.status, r.created_at, r.confidence_level, v.version_no
FROM calculation_runs r
JOIN project_versions v ON v.id = r.project_version_id
WHERE r.project_id = $1 AND r.kind = 'calculation'
ORDER BY r.created_at DESC
LIMIT $2;

-- name: ListProjectRuns :many
SELECT r.id, r.kind, r.project_version_id, v.version_no, r.input_hash,
    COALESCE(r.draft_hash, r.input_hash)::text AS draft_hash, r.match_version, r.econ_version,
    r.sim_version, r.seed, r.status, r.confidence_level, r.created_at,
    COALESCE(sj.id::text, '')::text AS job_id,
    COALESCE(CASE
        WHEN c.summary IS NULL THEN NULL
        WHEN r.kind = 'simulation' THEN jsonb_build_object(
            'variant_id', c.summary->'variant_id',
            'variant_hash', c.summary->'variant_hash',
            'variant_name', c.summary->'variant_name',
            'map_source', c.summary->'map_source',
            'verdict', c.summary->'result'->'verdict',
            'verdict_text', c.summary->'result'->'verdict_text',
            'throughput_per_h', c.summary->'result'->'kpi'->'throughput_per_h'->'median',
            'violation_rate', c.summary->'result'->'kpi'->'violation_rate'->'median'
        )
        ELSE jsonb_build_object(
            'solution_name', c.summary->'solution_name',
            'verification_flag', c.summary->'verification_flag',
            'variant_names', COALESCE((
                SELECT jsonb_agg(x->'name')
                FROM jsonb_array_elements(CASE WHEN jsonb_typeof(c.summary->'variants') = 'array'
                    THEN c.summary->'variants' ELSE '[]'::jsonb END) x
            ), '[]'::jsonb),
            'best_payback_years', (
                SELECT min((x->>'payback_years')::float8)
                FROM jsonb_array_elements(CASE WHEN jsonb_typeof(c.summary->'scenarios') = 'array'
                    THEN c.summary->'scenarios' ELSE '[]'::jsonb END) x
                WHERE x->>'kind' <> 'baseline' AND x->>'payback_years' IS NOT NULL
            )
        )
    END, '{}'::jsonb)::jsonb AS brief
FROM calculation_runs r
JOIN project_versions v ON v.id = r.project_version_id
LEFT JOIN calculation_results c ON c.run_id = r.id
LEFT JOIN LATERAL (
    SELECT j.id FROM simulation_jobs j WHERE j.run_id = r.id ORDER BY j.created_at DESC LIMIT 1
) sj ON true
WHERE r.project_id = $1
ORDER BY r.created_at DESC
LIMIT $2;

-- name: ListSharedCostItems :many
SELECT id, project_id, code, label, bucket, rub, sort_order, order_key
FROM shared_cost_items
WHERE project_id = $1
ORDER BY sort_order, code;

-- name: InsertSharedCostItem :one
INSERT INTO shared_cost_items (id, project_id, code, label, bucket, rub, sort_order, order_key)
VALUES (
    COALESCE(sqlc.narg(id)::uuid, gen_random_uuid()), sqlc.arg(project_id), sqlc.arg(code), sqlc.arg(label),
    sqlc.arg(bucket), sqlc.arg(rub), sqlc.arg(sort_order), sqlc.arg(order_key)
)
RETURNING id, project_id, code, label, bucket, rub, sort_order, order_key;

-- name: DeleteSharedCostItems :exec
DELETE FROM shared_cost_items
WHERE project_id = $1;

-- name: ListAssumptionSets :many
SELECT id, project_id, name, is_active, vat_rate, prices_include_vat, vat_recoverable, labor_cash_share, sort_order, order_key,
    discount_rate, utilization, availability, reserve, service_share, delivery_share, comm_rub_per_robot_year, technician_wage_month_rub
FROM assumption_sets
WHERE project_id = $1
ORDER BY sort_order, name;

-- name: InsertAssumptionSet :one
INSERT INTO assumption_sets (
    id, project_id, name, is_active, vat_rate, prices_include_vat, vat_recoverable, labor_cash_share, sort_order, order_key,
    discount_rate, utilization, availability, reserve, service_share, delivery_share, comm_rub_per_robot_year, technician_wage_month_rub
) VALUES (
    COALESCE(sqlc.narg(id)::uuid, gen_random_uuid()), sqlc.arg(project_id), sqlc.arg(name), sqlc.arg(is_active),
    sqlc.arg(vat_rate), sqlc.arg(prices_include_vat), sqlc.arg(vat_recoverable), sqlc.arg(labor_cash_share),
    sqlc.arg(sort_order), sqlc.arg(order_key), sqlc.arg(discount_rate), sqlc.narg(utilization), sqlc.narg(availability),
    sqlc.narg(reserve), sqlc.narg(service_share), sqlc.narg(delivery_share), sqlc.narg(comm_rub_per_robot_year),
    sqlc.narg(technician_wage_month_rub)
)
RETURNING id, project_id, name, is_active, vat_rate, prices_include_vat, vat_recoverable, labor_cash_share, sort_order, order_key,
    discount_rate, utilization, availability, reserve, service_share, delivery_share, comm_rub_per_robot_year, technician_wage_month_rub;

-- name: DeleteAssumptionSets :exec
DELETE FROM assumption_sets
WHERE project_id = $1;

-- name: InsertSimulationRun :one
INSERT INTO calculation_runs (
    project_id, project_version_id, input_hash, draft_hash, match_version, econ_version, sim_version, seed, status,
    kind, confidence_level
) VALUES (
    $1, $2, $3, $3, $4, $5, $6, $7, 'queued', 'simulation', $8
)
RETURNING id, project_id, project_version_id, input_hash, match_version, econ_version, sim_version, seed, status, created_at;

-- name: GetSimulationRun :one
SELECT r.id, r.project_id, r.project_version_id, r.input_hash,
    COALESCE(r.draft_hash, r.input_hash)::text AS draft_hash, r.match_version, r.econ_version,
    r.sim_version, r.seed, r.status, r.created_at, c.summary, r.confidence_level, v.version_no
FROM calculation_runs r
JOIN project_versions v ON v.id = r.project_version_id
LEFT JOIN calculation_results c ON c.run_id = r.id
WHERE r.id = $1 AND r.kind = 'simulation';

-- name: SyncSimulationRunStatus :exec
UPDATE calculation_runs r
SET status = j.status
FROM simulation_jobs j
WHERE j.id = $1 AND r.id = j.run_id AND r.kind = 'simulation';

-- name: CreateSimulationJob :one
INSERT INTO simulation_jobs (project_id, run_id, user_id, config, replications_total, max_attempts)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSimulationJob :one
SELECT *
FROM simulation_jobs
WHERE id = $1;

-- name: GetSimulationJobByRun :one
SELECT *
FROM simulation_jobs
WHERE run_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: ListSimulationJobsByProject :many
SELECT j.id, j.project_id, j.run_id, j.status, j.progress_pct, j.attempt, j.error_text, j.created_at,
    j.updated_at, j.canceled_at, j.config, j.replications_total, j.replications_done, j.started_at,
    j.finished_at, r.input_hash, COALESCE(r.draft_hash, r.input_hash)::text AS draft_hash, r.seed, r.sim_version,
    r.project_version_id,
    COALESCE(c.summary->>'variant_id', j.config->>'variant_id', '')::text AS variant_id,
    COALESCE(c.summary->>'variant_hash', '')::text AS variant_hash,
    jsonb_build_object(
        'verdict', c.summary->'result'->'verdict',
        'verdict_text', c.summary->'result'->'verdict_text',
        'kpi', c.summary->'result'->'kpi',
        'fleet', c.summary->'result'->'fleet',
        'variant_name', c.summary->'variant_name',
        'map_source', c.summary->'map_source'
    )::jsonb AS brief
FROM simulation_jobs j
JOIN calculation_runs r ON r.id = j.run_id
LEFT JOIN calculation_results c ON c.run_id = r.id
WHERE j.project_id = $1
ORDER BY j.created_at DESC
LIMIT $2;

-- name: ListRecentSimulationBriefs :many
SELECT r.id,
    COALESCE(c.summary->>'variant_id', '')::text AS variant_id,
    COALESCE(c.summary->>'variant_hash', '')::text AS variant_hash,
    COALESCE(c.summary->'econ_check', 'null'::jsonb)::jsonb AS econ_check
FROM calculation_runs r
JOIN calculation_results c ON c.run_id = r.id
WHERE r.project_id = $1 AND r.kind = 'simulation' AND r.status = 'succeeded'
ORDER BY r.created_at DESC
LIMIT $2;

-- name: CountActiveSimulationJobsByUser :one
SELECT count(*)::bigint
FROM simulation_jobs
WHERE user_id = $1 AND status IN ('queued', 'running');

-- name: CountActiveSimulationJobsByProject :one
SELECT count(*)::bigint
FROM simulation_jobs
WHERE project_id = $1 AND status IN ('queued', 'running');

-- name: ClaimSimulationJob :one
UPDATE simulation_jobs
SET status = 'running',
    attempt = attempt + 1,
    started_at = now(),
    heartbeat_at = now(),
    updated_at = now()
WHERE id = (
    SELECT q.id
    FROM simulation_jobs q
    WHERE q.status = 'queued' AND q.canceled_at IS NULL
    ORDER BY q.created_at, q.id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;

-- name: HeartbeatSimulationJob :one
UPDATE simulation_jobs
SET progress_pct = $2,
    replications_done = $3,
    heartbeat_at = now(),
    updated_at = now()
WHERE id = $1 AND status = 'running'
RETURNING canceled_at;

-- name: FinishSimulationJob :one
UPDATE simulation_jobs
SET status = sqlc.arg(status),
    progress_pct = CASE WHEN sqlc.arg(status) = 'succeeded' THEN 100 ELSE GREATEST(progress_pct, sqlc.arg(progress_pct)) END,
    replications_done = CASE
        WHEN sqlc.arg(status) = 'succeeded' THEN replications_total
        ELSE GREATEST(replications_done, sqlc.arg(replications_done))
    END,
    error_text = sqlc.narg(error_text),
    finished_at = now(),
    heartbeat_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'running'
RETURNING *;

-- name: RequeueSimulationJob :exec
UPDATE simulation_jobs
SET status = 'queued',
    attempt = CASE WHEN sqlc.arg(refund)::boolean THEN GREATEST(attempt - 1, 0) ELSE attempt END,
    error_text = sqlc.narg(error_text),
    heartbeat_at = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'running';

-- name: CancelSimulationJob :one
UPDATE simulation_jobs
SET canceled_at = COALESCE(canceled_at, now()),
    status = CASE WHEN status = 'queued' THEN 'canceled' ELSE status END,
    finished_at = CASE WHEN status = 'queued' THEN now() ELSE finished_at END,
    updated_at = now()
WHERE id = $1 AND status IN ('queued', 'running')
RETURNING *;

-- name: RetrySimulationJob :one
UPDATE simulation_jobs
SET status = 'queued',
    attempt = 0,
    progress_pct = 0,
    replications_done = 0,
    error_text = NULL,
    canceled_at = NULL,
    started_at = NULL,
    finished_at = NULL,
    heartbeat_at = NULL,
    updated_at = now()
WHERE id = $1 AND status IN ('failed', 'canceled')
RETURNING *;

-- name: RecoverStaleSimulationJobs :many
UPDATE simulation_jobs
SET status = CASE
        WHEN canceled_at IS NOT NULL THEN 'canceled'
        WHEN attempt >= max_attempts THEN 'failed'
        ELSE 'queued'
    END,
    error_text = CASE
        WHEN canceled_at IS NULL AND attempt >= max_attempts THEN sqlc.arg(failed_text)::text
        ELSE error_text
    END,
    finished_at = CASE
        WHEN canceled_at IS NOT NULL OR attempt >= max_attempts THEN now()
        ELSE NULL
    END,
    heartbeat_at = NULL,
    updated_at = now()
WHERE status = 'running' AND (heartbeat_at IS NULL OR heartbeat_at < sqlc.arg(stale_before)::timestamptz)
RETURNING id;

-- name: UpsertSimulationReplication :exec
INSERT INTO simulation_replications (run_id, idx, seed, status, metrics)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id, idx) DO UPDATE SET
    seed = EXCLUDED.seed,
    status = EXCLUDED.status,
    metrics = EXCLUDED.metrics,
    created_at = now();

-- name: DeleteSimulationReplications :exec
DELETE FROM simulation_replications
WHERE run_id = $1;

-- name: ListSimulationReplications :many
SELECT idx, seed, status, metrics
FROM simulation_replications
WHERE run_id = $1
ORDER BY idx;

-- name: UpsertSimulationArtifact :exec
INSERT INTO simulation_artifacts (run_id, kind, encoding, data, size_bytes, raw_bytes, event_count, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (run_id, kind) DO UPDATE SET
    encoding = EXCLUDED.encoding,
    data = EXCLUDED.data,
    size_bytes = EXCLUDED.size_bytes,
    raw_bytes = EXCLUDED.raw_bytes,
    event_count = EXCLUDED.event_count,
    expires_at = CASE WHEN simulation_artifacts.pinned THEN NULL ELSE EXCLUDED.expires_at END,
    created_at = now();

-- name: GetSimulationArtifact :one
SELECT id, run_id, kind, encoding, data, size_bytes, raw_bytes, event_count, pinned, expires_at, created_at
FROM simulation_artifacts
WHERE run_id = $1 AND kind = $2;

-- name: GetSimulationArtifactMeta :one
SELECT id, run_id, kind, encoding, size_bytes, raw_bytes, event_count, pinned, expires_at, created_at
FROM simulation_artifacts
WHERE run_id = $1 AND kind = $2;

-- name: SetSimulationArtifactPinned :one
UPDATE simulation_artifacts
SET pinned = sqlc.arg(pinned)::boolean,
    expires_at = CASE WHEN sqlc.arg(pinned)::boolean THEN NULL ELSE sqlc.narg(expires_at)::timestamptz END
WHERE run_id = sqlc.arg(run_id) AND kind = sqlc.arg(kind)
RETURNING pinned, expires_at;

-- name: DeleteExpiredSimulationArtifacts :execrows
DELETE FROM simulation_artifacts
WHERE NOT pinned AND expires_at IS NOT NULL AND expires_at < now();

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, csrf_token, expires_at, user_agent, ip_prefix)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at;

-- name: GetSessionByTokenHash :one
SELECT s.id, s.user_id, s.csrf_token, s.created_at, s.last_seen_at, s.expires_at, u.email, u.role
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL;

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = now()
WHERE id = $1;

-- name: ListUserSessions :many
SELECT id, created_at, last_seen_at, expires_at, user_agent, ip_prefix
FROM sessions
WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
ORDER BY last_seen_at DESC
LIMIT 100;

-- name: RevokeSession :execrows
UPDATE sessions
SET revoked_at = now()
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: RevokeOtherSessions :execrows
UPDATE sessions
SET revoked_at = now()
WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL;

-- name: RevokeUserSessions :execrows
UPDATE sessions
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: DeleteStaleSessions :execrows
DELETE FROM sessions
WHERE expires_at < sqlc.arg(before)::timestamptz OR revoked_at < sqlc.arg(before)::timestamptz;

-- name: LockProject :one
SELECT draft_seq
FROM projects
WHERE id = $1
FOR UPDATE;

-- name: GetProjectDraftSeq :one
SELECT draft_seq
FROM projects
WHERE id = $1;

-- name: SetProjectDraftSeq :exec
UPDATE projects
SET draft_seq = $2, updated_at = now()
WHERE id = $1;

-- name: SetProjectContent :exec
UPDATE projects
SET name = $2, object_type = $3, params = $4, updated_at = now()
WHERE id = $1;

-- name: InsertProjectOperation :exec
INSERT INTO project_operations (project_id, seq, tx_id, client_id, actor_user_id, label, ops, rejected_reason, rejected_details)
VALUES (
    sqlc.arg(project_id), sqlc.narg(seq), sqlc.arg(tx_id), sqlc.arg(client_id), sqlc.narg(actor_user_id), sqlc.arg(label),
    sqlc.arg(ops), sqlc.narg(rejected_reason), sqlc.narg(rejected_details)
);

-- name: ListProjectOperationsByTx :many
SELECT tx_id, seq, rejected_reason, rejected_details
FROM project_operations
WHERE project_id = sqlc.arg(project_id) AND tx_id = ANY(sqlc.arg(tx_ids)::uuid[]);

-- name: NotifyProjectOps :exec
SELECT pg_notify('project_ops', sqlc.arg(payload)::text);

-- name: ListSolutionIDs :many
SELECT id
FROM solutions;

-- name: DeleteProjectFleetItems :exec
DELETE FROM variant_fleet_items
WHERE variant_id IN (SELECT id FROM solution_variants WHERE project_id = $1);

-- name: DeleteProjectFinancing :exec
DELETE FROM financing_scenarios
WHERE variant_id IN (SELECT id FROM solution_variants WHERE project_id = $1);

-- name: ListProjectOperationsAfter :many
SELECT o.seq::bigint AS seq, o.tx_id, o.client_id, o.actor_user_id, u.email AS actor_email, o.label, o.ops, o.created_at
FROM project_operations o
LEFT JOIN users u ON u.id = o.actor_user_id
WHERE o.project_id = sqlc.arg(project_id) AND o.seq > sqlc.arg(after)::bigint
ORDER BY o.seq
LIMIT sqlc.arg(max_rows);

-- name: DeleteOldProjectOperations :execrows
DELETE FROM project_operations
WHERE created_at < sqlc.arg(before)::timestamptz;

-- name: SessionActive :one
SELECT EXISTS (
    SELECT 1 FROM sessions
    WHERE id = sqlc.arg(id)
      AND revoked_at IS NULL
      AND expires_at > now()
      AND last_seen_at > now() - make_interval(secs => sqlc.arg(idle_seconds)::int)
);

-- name: NotifyProjectPresence :exec
SELECT pg_notify('project_presence', sqlc.arg(payload)::text);

-- name: RecordAuditEvent :exec
INSERT INTO audit_events (actor_user_id, action, target_type, target_id, request_id, meta)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListAuditEvents :many
SELECT a.id, a.at, a.actor_user_id, u.email AS actor_email, a.action, a.target_type, a.target_id, a.meta,
    p.name AS project_name, s.name AS solution_name
FROM audit_events a
LEFT JOIN users u ON u.id = a.actor_user_id
LEFT JOIN projects p ON a.target_type = 'project' AND p.id::text = a.target_id
LEFT JOIN solutions s ON a.target_type = 'solution' AND s.id::text = a.target_id
WHERE (sqlc.narg(target_type)::text IS NULL OR a.target_type = sqlc.narg(target_type)::text)
    AND (sqlc.narg(target_id)::text IS NULL OR a.target_id = sqlc.narg(target_id)::text)
    AND (sqlc.narg(actions)::text[] IS NULL OR a.action = ANY(sqlc.narg(actions)::text[]))
    AND (sqlc.narg(before_id)::bigint IS NULL
        OR (a.at, a.id) < (SELECT b.at, b.id FROM audit_events b WHERE b.id = sqlc.narg(before_id)::bigint))
ORDER BY a.at DESC, a.id DESC
LIMIT sqlc.arg(max_rows);

-- name: DeleteOldAuditEvents :execrows
DELETE FROM audit_events
WHERE at < sqlc.arg(before)::timestamptz;

-- name: TrashProject :one
UPDATE projects
SET deleted_at = now(), deleted_by = sqlc.arg(actor)::uuid, updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING deleted_at;

-- name: GetTrashedProject :one
SELECT id, user_id, name, object_type, deleted_at, deleted_by
FROM projects
WHERE id = $1 AND deleted_at IS NOT NULL;

-- name: RestoreProject :one
UPDATE projects
SET deleted_at = NULL, deleted_by = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NOT NULL AND deleted_at >= sqlc.arg(kept_since)::timestamptz
RETURNING id;

-- name: ListTrashedProjects :many
SELECT id, name, object_type, deleted_at, created_at
FROM projects
WHERE user_id = sqlc.arg(user_id)::uuid AND deleted_at IS NOT NULL
ORDER BY deleted_at DESC
LIMIT sqlc.arg(max_rows);

-- name: PurgeProject :execrows
DELETE FROM projects
WHERE id = $1 AND deleted_at IS NOT NULL;

-- name: PurgeExpiredProjects :many
DELETE FROM projects
WHERE id IN (
    SELECT id FROM projects
    WHERE deleted_at < sqlc.arg(before)::timestamptz
    ORDER BY deleted_at
    LIMIT sqlc.arg(max_rows)
)
RETURNING id;

-- name: CancelProjectSimulationJobs :many
UPDATE simulation_jobs
SET canceled_at = COALESCE(canceled_at, now()),
    status = CASE WHEN status = 'queued' THEN 'canceled' ELSE status END,
    finished_at = CASE WHEN status = 'queued' THEN now() ELSE finished_at END,
    updated_at = now()
WHERE project_id = $1 AND status IN ('queued', 'running')
RETURNING id;

-- name: CreateEmailToken :one
INSERT INTO email_tokens (user_id, purpose, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at;

-- name: UseEmailToken :one
UPDATE email_tokens
SET used_at = now()
WHERE token_hash = sqlc.arg(token_hash) AND purpose = sqlc.arg(purpose)
    AND used_at IS NULL AND expires_at > now()
RETURNING user_id;

-- name: DeleteUserEmailTokens :exec
DELETE FROM email_tokens
WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL;

-- name: DeleteStaleEmailTokens :execrows
DELETE FROM email_tokens
WHERE expires_at < sqlc.arg(before)::timestamptz;

-- name: MarkEmailVerified :execrows
UPDATE users
SET email_verified_at = COALESCE(email_verified_at, now())
WHERE id = $1;

-- name: SetUserPassword :execrows
UPDATE users
SET password_hash = $2, password_changed_at = now()
WHERE id = $1;

-- name: EnqueueEmail :one
INSERT INTO email_outbox (to_email, template, payload)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ClaimEmailOutbox :many
UPDATE email_outbox
SET status = 'queued', attempts = attempts + 1, next_attempt_at = now() + interval '10 minutes'
WHERE id IN (
    SELECT id FROM email_outbox
    WHERE status = 'queued' AND next_attempt_at <= now()
    ORDER BY next_attempt_at
    LIMIT sqlc.arg(max_rows)
    FOR UPDATE SKIP LOCKED
)
RETURNING id, to_email, template, payload, attempts;

-- name: MarkEmailSent :exec
UPDATE email_outbox
SET status = 'sent', sent_at = now(), error_text = NULL,
    payload = CASE WHEN sqlc.arg(keep_link)::boolean THEN payload ELSE payload - 'link' END
WHERE id = sqlc.arg(id);

-- name: MarkEmailFailed :exec
UPDATE email_outbox
SET status = CASE WHEN attempts >= sqlc.arg(max_attempts)::int THEN 'failed' ELSE 'queued' END,
    next_attempt_at = now() + make_interval(mins => attempts * attempts),
    error_text = sqlc.arg(error_text)
WHERE id = sqlc.arg(id);

-- name: ListOutboxByEmail :many
SELECT id, to_email, template, payload, status, attempts, created_at, sent_at
FROM email_outbox
WHERE to_email = $1
ORDER BY created_at DESC
LIMIT sqlc.arg(max_rows);

-- name: DeleteOldEmails :execrows
DELETE FROM email_outbox
WHERE created_at < sqlc.arg(before)::timestamptz AND status <> 'queued';

-- name: CreateInvitation :one
INSERT INTO invitations (email, token_hash, invited_by, project_id, project_role, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, email, project_id, project_role, created_at, expires_at;

-- name: GetInvitationByTokenHash :one
SELECT i.id, i.email, i.project_id, i.project_role, i.expires_at, i.accepted_at, i.revoked_at, p.name AS project_name
FROM invitations i
LEFT JOIN projects p ON p.id = i.project_id
WHERE i.token_hash = $1;

-- name: AcceptInvitation :one
UPDATE invitations
SET accepted_at = now(), accepted_user_id = sqlc.arg(user_id)::uuid
WHERE id = sqlc.arg(id) AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()
RETURNING id, project_id, project_role;

-- name: ListOpenInvitationsForEmail :many
SELECT id, project_id, project_role, invited_by
FROM invitations
WHERE lower(email) = lower(sqlc.arg(email)::text)
    AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()
ORDER BY created_at;

-- name: ListInvitations :many
SELECT i.id, i.email, i.project_id, p.name AS project_name, i.project_role, i.created_at, i.expires_at,
    i.accepted_at, i.revoked_at, u.email AS invited_by_email
FROM invitations i
LEFT JOIN projects p ON p.id = i.project_id
LEFT JOIN users u ON u.id = i.invited_by
WHERE (sqlc.narg(project_id)::uuid IS NULL OR i.project_id = sqlc.narg(project_id)::uuid)
ORDER BY i.created_at DESC
LIMIT sqlc.arg(max_rows);

-- name: RevokeInvitation :execrows
UPDATE invitations
SET revoked_at = now()
WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: GetInvitation :one
SELECT id, email, project_id, project_role, invited_by, accepted_at, revoked_at
FROM invitations
WHERE id = $1;
