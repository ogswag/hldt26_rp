ALTER TABLE catalog_revisions
    ADD COLUMN content_sha256 TEXT,
    ADD COLUMN published_at TIMESTAMPTZ;

UPDATE catalog_revisions
SET published_at = created_at
WHERE status = 'published' AND published_at IS NULL;

CREATE TABLE catalog_revision_items (
    revision_id UUID NOT NULL REFERENCES catalog_revisions (id) ON DELETE CASCADE,
    solution_id UUID NOT NULL REFERENCES solutions (id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    vendor TEXT,
    kind TEXT,
    subtype TEXT,
    status TEXT,
    industry TEXT,
    scenario TEXT,
    price_rub NUMERIC(15, 2),
    source_url TEXT,
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
    sourced_at TIMESTAMPTZ,
    family TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    object_types JSONB NOT NULL DEFAULT '[]'::jsonb,
    uses JSONB NOT NULL DEFAULT '[]'::jsonb,
    capability_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (revision_id, solution_id)
);

CREATE INDEX catalog_revision_items_revision_name_idx
    ON catalog_revision_items (revision_id, name);

CREATE INDEX catalog_revision_items_revision_kind_idx
    ON catalog_revision_items (revision_id, kind);

CREATE TABLE dictionary_entries (
    kind TEXT NOT NULL,
    code TEXT NOT NULL,
    label TEXT NOT NULL,
    object_type TEXT,
    PRIMARY KEY (kind, code)
);

CREATE TABLE capability_task_rules (
    capability_code TEXT NOT NULL,
    task_code TEXT NOT NULL,
    relation TEXT NOT NULL,
    PRIMARY KEY (capability_code, task_code)
);

INSERT INTO dictionary_entries (kind, code, label, object_type) VALUES
    ('task', 'pallet_inbound', 'Приемка паллет', 'warehouse'),
    ('task', 'pallet_putaway', 'Размещение паллет', 'warehouse'),
    ('task', 'pallet_outbound', 'Отгрузка паллет', 'warehouse'),
    ('task', 'pallet_move', 'Паллетное перемещение', 'warehouse'),
    ('task', 'piece_pick', 'Мелкоштучный отбор', 'warehouse'),
    ('task', 'cleaning', 'Уборка', 'warehouse'),
    ('capability', 'warehouse_indoor', 'Наземная работа на складе', 'warehouse'),
    ('capability', 'payload_pallet', 'Паллетная нагрузка', 'warehouse'),
    ('capability', 'payload_unit', 'Штучная или контейнерная нагрузка', 'warehouse'),
    ('capability', 'aisle_rated', 'Есть ширина или минимальный проезд', 'warehouse'),
    ('capability', 'temp_rated', 'Есть рабочий диапазон температур', 'warehouse'),
    ('capability', 'cleaning', 'Уборка помещений', 'warehouse'),
    ('capability', 'amr', 'Автономный мобильный робот', 'warehouse'),
    ('capability', 'stacker', 'Штабелер', 'warehouse'),
    ('capability', 'forklift', 'Погрузчик', 'warehouse');

INSERT INTO capability_task_rules (capability_code, task_code, relation) VALUES
    ('payload_pallet', 'pallet_inbound', 'required'),
    ('payload_pallet', 'pallet_putaway', 'required'),
    ('payload_pallet', 'pallet_outbound', 'required'),
    ('payload_pallet', 'pallet_move', 'required'),
    ('payload_unit', 'piece_pick', 'required'),
    ('cleaning', 'cleaning', 'required');

CREATE FUNCTION catalog_revision_items_reject_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'published catalog revision items are immutable';
END;
$$;

CREATE TRIGGER catalog_revision_items_immutable
    BEFORE UPDATE OR DELETE ON catalog_revision_items
    FOR EACH ROW
    EXECUTE FUNCTION catalog_revision_items_reject_mutation();
