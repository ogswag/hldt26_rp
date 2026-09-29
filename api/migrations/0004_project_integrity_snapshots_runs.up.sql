CREATE TABLE catalog_revisions (
    id UUID PRIMARY KEY,
    revision_no INTEGER NOT NULL UNIQUE,
    status TEXT NOT NULL,
    code TEXT NOT NULL UNIQUE,
    comment TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO catalog_revisions (id, revision_no, status, code, comment)
VALUES (
    '11111111-1111-4111-8111-111111111111',
    1,
    'published',
    'catalog-v4-seed',
    'Initial published revision of catalog_export_v4 plus robot_specs.json'
);

ALTER TABLE projects
    ADD COLUMN status TEXT NOT NULL DEFAULT 'draft',
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN current_run_id UUID,
    ADD COLUMN catalog_revision_id UUID REFERENCES catalog_revisions (id),
    ADD COLUMN input_hash TEXT NOT NULL DEFAULT '';

UPDATE projects
SET catalog_revision_id = '11111111-1111-4111-8111-111111111111'
WHERE catalog_revision_id IS NULL;

CREATE TABLE project_drafts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL UNIQUE REFERENCES projects (id) ON DELETE CASCADE,
    document JSONB NOT NULL,
    input_hash TEXT NOT NULL,
    catalog_revision_id UUID REFERENCES catalog_revisions (id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE project_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    version_no INTEGER NOT NULL,
    snapshot JSONB NOT NULL,
    input_hash TEXT NOT NULL,
    catalog_revision_id UUID REFERENCES catalog_revisions (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, version_no)
);

CREATE TABLE project_processes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    task_type TEXT NOT NULL,
    is_baseline BOOLEAN NOT NULL DEFAULT true,
    demand JSONB NOT NULL DEFAULT '{}'::jsonb,
    sla JSONB NOT NULL DEFAULT '{}'::jsonb,
    point_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    durations JSONB NOT NULL DEFAULT '{}'::jsonb,
    baseline_staff JSONB NOT NULL DEFAULT '{}'::jsonb,
    sort_order INTEGER NOT NULL DEFAULT 0,
    UNIQUE (project_id, code)
);

CREATE INDEX project_processes_project_id_sort_idx
    ON project_processes (project_id, sort_order);

CREATE TABLE solution_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    notes TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX solution_variants_project_id_sort_idx
    ON solution_variants (project_id, sort_order);

CREATE TABLE variant_fleet_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES solution_variants (id) ON DELETE CASCADE,
    solution_id UUID REFERENCES solutions (id),
    quantity INTEGER NOT NULL,
    task_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    price_override_rub NUMERIC(15, 2),
    price_override_reason TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE financing_scenarios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES solution_variants (id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    tariff TEXT,
    assumptions JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE calculation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    project_version_id UUID NOT NULL REFERENCES project_versions (id),
    input_hash TEXT NOT NULL,
    match_version TEXT NOT NULL,
    econ_version TEXT NOT NULL,
    sim_version TEXT NOT NULL,
    seed INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX calculation_runs_project_id_created_idx
    ON calculation_runs (project_id, created_at DESC);

CREATE TABLE calculation_results (
    run_id UUID PRIMARY KEY REFERENCES calculation_runs (id) ON DELETE CASCADE,
    summary JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE simulation_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    run_id UUID REFERENCES calculation_runs (id),
    status TEXT NOT NULL DEFAULT 'queued',
    progress_pct INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0,
    error_text TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    canceled_at TIMESTAMPTZ
);
