CREATE TABLE calc_norms (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    values JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by UUID REFERENCES users (id)
);

INSERT INTO calc_norms (id, values) VALUES (1, '{}'::jsonb);
