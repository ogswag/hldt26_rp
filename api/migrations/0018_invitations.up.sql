CREATE TABLE invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    invited_by UUID REFERENCES users (id) ON DELETE SET NULL,
    project_id UUID REFERENCES projects (id) ON DELETE CASCADE,
    project_role TEXT CHECK (project_role IN ('editor', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    accepted_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT invitations_project_role CHECK ((project_id IS NULL) = (project_role IS NULL))
);

CREATE INDEX invitations_email_idx ON invitations (lower(email), created_at DESC);
CREATE INDEX invitations_open_idx ON invitations (expires_at) WHERE accepted_at IS NULL AND revoked_at IS NULL;
