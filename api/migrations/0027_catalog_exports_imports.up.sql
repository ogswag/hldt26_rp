-- An export keeps the catalog as it was written to the file: the base of a three-way merge on upload.
CREATE TABLE catalog_exports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    snapshot JSONB NOT NULL
);

-- An upload session: the file's cells, the column mapping, then what was applied.
CREATE TABLE catalog_imports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    file_name TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('robot', 'catalog')),
    export_id UUID REFERENCES catalog_exports (id) ON DELETE SET NULL,
    grid JSONB NOT NULL,
    mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'photos', 'done')),
    summary JSONB NOT NULL DEFAULT '{}'::jsonb
);
