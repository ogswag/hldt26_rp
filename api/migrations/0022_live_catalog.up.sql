-- Calculations read the live catalog; each run keeps the checksum of the catalog it used in its summary.
ALTER TABLE projects DROP COLUMN catalog_revision_id;
ALTER TABLE project_drafts DROP COLUMN catalog_revision_id;
ALTER TABLE project_versions DROP COLUMN catalog_revision_id;

DROP TABLE catalog_revision_items;
DROP TABLE catalog_revisions;
DROP FUNCTION catalog_revision_items_reject_mutation();
DROP FUNCTION catalog_revision_reject_identity_mutation();

-- The input hash no longer covers a catalog revision; the API rehashes runs on the project's next draft save.
UPDATE calculation_runs SET draft_hash = NULL;
