DROP TRIGGER IF EXISTS catalog_revision_identity_immutable ON catalog_revisions;
DROP FUNCTION IF EXISTS catalog_revision_reject_identity_mutation();

DROP TRIGGER IF EXISTS catalog_revision_items_immutable ON catalog_revision_items;

CREATE OR REPLACE FUNCTION catalog_revision_items_reject_mutation()
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
