-- Project events for live clients (SSE). Triggers cover every writer: API instances, workers and SQL.
CREATE FUNCTION notify_project_event(project UUID, kind TEXT, member UUID) RETURNS VOID
LANGUAGE sql
AS $$
    SELECT pg_notify('project_events', json_build_object('project_id', project, 'type', kind, 'user_id', member)::text)
$$;

CREATE FUNCTION project_runs_changed() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM notify_project_event(NEW.project_id, 'runs', NULL);
    RETURN NULL;
END
$$;

CREATE TRIGGER calculation_runs_event AFTER INSERT ON calculation_runs
    FOR EACH ROW EXECUTE FUNCTION project_runs_changed();
CREATE TRIGGER simulation_jobs_event AFTER INSERT OR UPDATE OF status ON simulation_jobs
    FOR EACH ROW EXECUTE FUNCTION project_runs_changed();

CREATE FUNCTION project_members_changed() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM notify_project_event(OLD.project_id, 'access', OLD.user_id);
    ELSE
        PERFORM notify_project_event(NEW.project_id, 'access', NEW.user_id);
    END IF;
    RETURN NULL;
END
$$;

CREATE TRIGGER project_members_event AFTER INSERT OR UPDATE OR DELETE ON project_members
    FOR EACH ROW EXECUTE FUNCTION project_members_changed();

CREATE FUNCTION project_deleted() RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM notify_project_event(OLD.id, 'deleted', NULL);
    RETURN NULL;
END
$$;

CREATE TRIGGER projects_deleted_event AFTER DELETE ON projects
    FOR EACH ROW EXECUTE FUNCTION project_deleted();
CREATE TRIGGER projects_trashed_event AFTER UPDATE OF deleted_at ON projects
    FOR EACH ROW WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL) EXECUTE FUNCTION project_deleted();
