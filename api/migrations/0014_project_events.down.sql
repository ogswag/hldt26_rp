DROP TRIGGER projects_trashed_event ON projects;
DROP TRIGGER projects_deleted_event ON projects;
DROP FUNCTION project_deleted();
DROP TRIGGER project_members_event ON project_members;
DROP FUNCTION project_members_changed();
DROP TRIGGER simulation_jobs_event ON simulation_jobs;
DROP TRIGGER calculation_runs_event ON calculation_runs;
DROP FUNCTION project_runs_changed();
DROP FUNCTION notify_project_event(UUID, TEXT, UUID);
