-- One photo per robot, already scaled down by the API.
CREATE TABLE solution_images (
    solution_id UUID PRIMARY KEY REFERENCES solutions (id) ON DELETE CASCADE,
    content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png')),
    bytes BYTEA NOT NULL,
    sha256 TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
