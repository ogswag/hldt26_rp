-- Brings back the revision tables and columns empty; the API of 0021 freezes the seed revision on start.
CREATE TABLE catalog_revisions (
    id UUID PRIMARY KEY,
    revision_no INTEGER NOT NULL UNIQUE,
    status TEXT NOT NULL,
    code TEXT NOT NULL UNIQUE,
    comment TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    content_sha256 TEXT,
    published_at TIMESTAMPTZ
);

INSERT INTO catalog_revisions (id, revision_no, status, code, comment, published_at)
VALUES (
    '11111111-1111-4111-8111-111111111111',
    1,
    'published',
    'catalog-v4-seed',
    'Initial published revision of catalog_export_v4 plus robot_specs.json',
    now()
);

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

CREATE FUNCTION catalog_revision_items_reject_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    target_revision_id UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_revision_id := OLD.revision_id;
    ELSE
        target_revision_id := NEW.revision_id;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM catalog_revisions
        WHERE id = target_revision_id
          AND content_sha256 IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'frozen catalog revision items are immutable';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER catalog_revision_items_immutable
    BEFORE INSERT OR UPDATE OR DELETE ON catalog_revision_items
    FOR EACH ROW
    EXECUTE FUNCTION catalog_revision_items_reject_mutation();

CREATE FUNCTION catalog_revision_reject_identity_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.content_sha256 IS NOT NULL THEN
        IF TG_OP = 'DELETE' THEN
            RAISE EXCEPTION 'frozen catalog revision is immutable';
        END IF;
        IF NEW.revision_no IS DISTINCT FROM OLD.revision_no
            OR NEW.code IS DISTINCT FROM OLD.code
            OR NEW.content_sha256 IS DISTINCT FROM OLD.content_sha256
            OR NEW.published_at IS DISTINCT FROM OLD.published_at THEN
            RAISE EXCEPTION 'frozen catalog revision identity is immutable';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER catalog_revision_identity_immutable
    BEFORE UPDATE OR DELETE ON catalog_revisions
    FOR EACH ROW
    EXECUTE FUNCTION catalog_revision_reject_identity_mutation();

ALTER TABLE projects ADD COLUMN catalog_revision_id UUID REFERENCES catalog_revisions (id);
ALTER TABLE project_drafts ADD COLUMN catalog_revision_id UUID REFERENCES catalog_revisions (id);
ALTER TABLE project_versions ADD COLUMN catalog_revision_id UUID REFERENCES catalog_revisions (id);
UPDATE projects SET catalog_revision_id = '11111111-1111-4111-8111-111111111111';
