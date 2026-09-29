DROP TRIGGER IF EXISTS catalog_revision_items_immutable ON catalog_revision_items;
DROP FUNCTION IF EXISTS catalog_revision_items_reject_mutation();
DROP TABLE IF EXISTS capability_task_rules;
DROP TABLE IF EXISTS dictionary_entries;
DROP TABLE IF EXISTS catalog_revision_items;
ALTER TABLE catalog_revisions
    DROP COLUMN IF EXISTS content_sha256,
    DROP COLUMN IF EXISTS published_at;
