package httpserver

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
)

// accessLevel is what a route needs from the caller's role in the project.
type accessLevel int

const (
	accessRead accessLevel = iota + 1
	accessWrite
	accessOwner
)

const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

func roleAllows(role string, need accessLevel) bool {
	switch role {
	case RoleOwner:
		return true
	case RoleEditor:
		return need <= accessWrite
	case RoleViewer:
		return need <= accessRead
	}
	return false
}

// ValidMemberRole reports whether role can be granted to a project member.
func ValidMemberRole(role string) bool {
	return role == RoleEditor || role == RoleViewer
}

type projectAccess struct {
	row  db.GetProjectRow
	role string
}

type accessKey struct{}

func withProjectAccess(ctx context.Context, a projectAccess) context.Context {
	return context.WithValue(ctx, accessKey{}, a)
}

// projectFrom returns the project a scoped route resolved. It panics on an unscoped route.
func projectFrom(r *http.Request) db.GetProjectRow {
	return r.Context().Value(accessKey{}).(projectAccess).row
}

// roleFrom returns the caller's role in the project of a scoped route.
func roleFrom(r *http.Request) string {
	return r.Context().Value(accessKey{}).(projectAccess).role
}

// scoped registers a route under {id}, {runId} or {jobId} behind one project access check.
func (s *Server) scoped(r chi.Router, method, pattern string, need accessLevel, h http.HandlerFunc) {
	s.scopedRoutes[method+" "+pattern] = need
	r.Method(method, pattern, s.withAccess(need, h))
}

// scopedTrash registers an owner route on a project that is in the trash, which withAccess leaves out.
func (s *Server) scopedTrash(r chi.Router, method, pattern string, h http.HandlerFunc) {
	s.scopedRoutes[method+" "+pattern] = accessOwner
	r.Method(method, pattern, s.withTrashAccess(h))
}

type trashKey struct{}

// trashedFrom returns the trashed project a scopedTrash route resolved.
func trashedFrom(r *http.Request) db.GetTrashedProjectRow {
	return r.Context().Value(trashKey{}).(db.GetTrashedProjectRow)
}

func (s *Server) withTrashAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guestForbidden(w, r) {
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор проекта.")
			return
		}
		row, err := s.q.GetTrashedProject(r.Context(), id)
		if err != nil {
			if isNoRows(err) {
				writeError(w, r, http.StatusNotFound, "Проект не найден.")
				return
			}
			s.internal(w, r, "access.trash", err, "Не удалось проверить доступ к проекту. Повторите запрос.")
			return
		}
		if row.UserID == nil || *row.UserID != auth.FromRequest(r).UserID {
			writeError(w, r, http.StatusNotFound, "Проект не найден.")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), trashKey{}, row)))
	}
}

func (s *Server) withAccess(need accessLevel, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if guestForbidden(w, r) {
			return
		}
		projectID, notFound, ok := s.scopeProjectID(w, r)
		if !ok {
			return
		}
		ctx := r.Context()
		row, err := s.q.GetProjectAccess(ctx, db.GetProjectAccessParams{ProjectID: projectID, UserID: auth.FromRequest(r).UserID})
		if err != nil {
			if isNoRows(err) {
				writeError(w, r, http.StatusNotFound, notFound)
				return
			}
			s.internal(w, r, "access.project", err, "Не удалось проверить доступ к проекту. Повторите запрос.")
			return
		}
		if row.DeletedAt.Valid {
			writeCode(w, r, http.StatusGone, "project_deleted", "Проект в корзине. Восстановите его, чтобы продолжить.")
			return
		}
		if !roleAllows(row.Role, need) {
			writeJSON(w, http.StatusForbidden, errorBody{Error: forbiddenText(need), Code: "forbidden", RequestID: requestID(r)})
			return
		}
		a := projectAccess{row: projectRow(row), role: row.Role}
		next(w, r.WithContext(withProjectAccess(ctx, a)))
	}
}

func forbiddenText(need accessLevel) string {
	if need == accessOwner {
		return "Это может сделать только владелец проекта."
	}
	return "У вас доступ только для просмотра. Попросите владельца выдать права на редактирование."
}

// scopeProjectID finds the project of the route from its id, run id or job id.
func (s *Server) scopeProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	ctx := r.Context()
	if raw := chi.URLParam(r, "id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор проекта.")
			return uuid.Nil, "", false
		}
		return id, "Проект не найден.", true
	}
	if raw := chi.URLParam(r, "runId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор запуска.")
			return uuid.Nil, "", false
		}
		pid, err := s.q.GetRunProjectID(ctx, id)
		return s.scopeLookup(w, r, pid, err, "Запуск не найден.")
	}
	if raw := chi.URLParam(r, "jobId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор задания.")
			return uuid.Nil, "", false
		}
		pid, err := s.q.GetJobProjectID(ctx, id)
		return s.scopeLookup(w, r, pid, err, "Задание не найдено.")
	}
	writeError(w, r, http.StatusNotFound, "Проект не найден.")
	return uuid.Nil, "", false
}

func (s *Server) scopeLookup(w http.ResponseWriter, r *http.Request, pid uuid.UUID, err error, notFound string) (uuid.UUID, string, bool) {
	if err == nil {
		return pid, notFound, true
	}
	if isNoRows(err) {
		writeError(w, r, http.StatusNotFound, notFound)
	} else {
		s.internal(w, r, "access.scope", err, "Не удалось проверить доступ к проекту. Повторите запрос.")
	}
	return uuid.Nil, "", false
}

func projectRow(a db.GetProjectAccessRow) db.GetProjectRow {
	return db.GetProjectRow{
		ID: a.ID, UserID: a.UserID, Name: a.Name, ObjectType: a.ObjectType, Params: a.Params, Results: a.Results,
		ModelVersion: a.ModelVersion, CalcSeed: a.CalcSeed, CreatedAt: a.CreatedAt, Status: a.Status,
		UpdatedAt: a.UpdatedAt, DeletedAt: a.DeletedAt, CurrentRunID: a.CurrentRunID,
		InputHash: a.InputHash,
	}
}
