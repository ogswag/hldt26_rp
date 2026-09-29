CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE solutions (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    vendor TEXT,
    kind TEXT,
    subtype TEXT,
    status TEXT,
    industry TEXT,
    scenario TEXT,
    price_rub NUMERIC(15, 2),
    source_url TEXT,
    raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE solution_specs (
    solution_id UUID PRIMARY KEY REFERENCES solutions (id) ON DELETE CASCADE,
    payload_kg NUMERIC,
    mass_kg NUMERIC,
    length_mm NUMERIC,
    width_mm NUMERIC,
    height_mm NUMERIC,
    speed_mps NUMERIC,
    endurance_h NUMERIC,
    charge_min NUMERIC,
    nav_type TEXT,
    pos_accuracy_mm NUMERIC,
    min_aisle_mm NUMERIC,
    temp_min_c NUMERIC,
    temp_max_c NUMERIC,
    lifetime_years NUMERIC,
    service_pct_year NUMERIC,
    confidence TEXT,
    sourced_at TIMESTAMPTZ
);

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users (id),
    name TEXT NOT NULL,
    object_type TEXT NOT NULL,
    params JSONB NOT NULL DEFAULT '{}'::jsonb,
    model_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE scenarios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    solution_id UUID REFERENCES solutions (id),
    inputs JSONB NOT NULL DEFAULT '{}'::jsonb,
    results JSONB NOT NULL DEFAULT '{}'::jsonb
);
