DROP TRIGGER projects_restored_event ON projects;
DROP FUNCTION project_restored();
DROP INDEX projects_trash_idx;
ALTER TABLE projects DROP COLUMN deleted_by;
