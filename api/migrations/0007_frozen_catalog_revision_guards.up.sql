DROP TRIGGER IF EXISTS catalog_revision_items_immutable ON catalog_revision_items;

CREATE OR REPLACE FUNCTION catalog_revision_items_reject_mutation()
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
