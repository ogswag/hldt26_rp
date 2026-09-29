ALTER TABLE projects ADD COLUMN deleted_by UUID REFERENCES users (id) ON DELETE SET NULL;

CREATE INDEX projects_trash_idx ON projects (deleted_at) WHERE deleted_at IS NOT NULL;

CREATE FUNCTION project_restored() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM notify_project_event(NEW.id, 'reset', NULL);
    RETURN NULL;
END
$$;

CREATE TRIGGER projects_restored_event AFTER UPDATE OF deleted_at ON projects
    FOR EACH ROW WHEN (OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL) EXECUTE FUNCTION project_restored();
