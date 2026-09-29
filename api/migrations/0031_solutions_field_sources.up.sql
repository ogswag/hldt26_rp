-- Where each value of a robot comes from: field code -> {source_url, confidence, note, sourced_at}.
-- The card keeps its own source_url and confidence for values with no entry.
ALTER TABLE solutions ADD COLUMN field_sources JSONB NOT NULL DEFAULT '{}'::jsonb;
