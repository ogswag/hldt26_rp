package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
)

const trashListCap = 200

type trashItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ObjectType string     `json:"object_type"`
	DeletedAt  *time.Time `json:"deleted_at"`
	PurgeAt    *time.Time `json:"purge_at"`
	CreatedAt  *time.Time `json:"created_at"`
}

type deletedJSON struct {
	DeletedAt *time.Time `json:"deleted_at"`
	PurgeAt   *time.Time `json:"purge_at"`
}

func (s *Server) purgeAt(deleted pgtype.Timestamptz) *time.Time {
	if !deleted.Valid {
		return nil
	}
	at := deleted.Time.Add(s.trashRetention())
	return &at
}

func (s *Server) trashRetention() time.Duration {
	if s.cfg.TrashRetention <= 0 {
		return 30 * 24 * time.Hour
	}
	return s.cfg.TrashRetention
}

// deleteProject moves the project to the trash, where the owner can restore it until it is purged.
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	id := auth.FromRequest(r).UserID
	ctx := r.Context()
	var deleted pgtype.Timestamptz
	err := s.inTx(ctx, func(tx *Server) error {
		var err error
		deleted, err = tx.q.TrashProject(ctx, db.TrashProjectParams{ID: row.ID, Actor: id})
		if err != nil {
			return err
		}
		jobs, err := tx.q.CancelProjectSimulationJobs(ctx, row.ID)
		if err != nil {
			return err
		}
		for _, jobID := range jobs {
			if err := tx.q.SyncSimulationRunStatus(ctx, jobID); err != nil {
				return err
			}
		}
		return tx.recordAudit(ctx, requestID(r), audit.Event{
			Actor: &id, Action: audit.ActionProjectDelete, TargetType: audit.TargetProject, TargetID: row.ID.String(),
			Meta: map[string]any{"jobs_canceled": len(jobs)},
		})
	})
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusNotFound, "Проект не найден.")
			return
		}
		s.internal(w, r, "projects.delete", err, "Не удалось удалить проект. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, deletedJSON{DeletedAt: timestamptzPtr(deleted), PurgeAt: s.purgeAt(deleted)})
}

func (s *Server) listTrash(w http.ResponseWriter, r *http.Request) {
	id, ok := signedIn(w, r)
	if !ok {
		return
	}
	rows, err := s.q.ListTrashedProjects(r.Context(), db.ListTrashedProjectsParams{UserID: id.UserID, MaxRows: trashListCap})
	if err != nil {
		s.internal(w, r, "projects.trash", err, "Не удалось загрузить корзину. Повторите запрос.")
		return
	}
	items := make([]trashItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, trashItem{
			ID:         row.ID.String(),
			Name:       row.Name,
			ObjectType: row.ObjectType,
			DeletedAt:  timestamptzPtr(row.DeletedAt),
			PurgeAt:    s.purgeAt(row.DeletedAt),
			CreatedAt:  timestamptzPtr(row.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) restoreProject(w http.ResponseWriter, r *http.Request) {
	row := trashedFrom(r)
	actor := auth.FromRequest(r).UserID
	kept := time.Now().Add(-s.trashRetention())
	err := s.inTx(r.Context(), func(tx *Server) error {
		if _, err := tx.q.RestoreProject(r.Context(), db.RestoreProjectParams{
			ID: row.ID, KeptSince: pgtype.Timestamptz{Time: kept, Valid: true},
		}); err != nil {
			return err
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionProjectRestor, TargetType: audit.TargetProject, TargetID: row.ID.String(),
		})
	})
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusGone, "Проект удалён больше 30 дней назад и восстановлению не подлежит.")
			return
		}
		s.internal(w, r, "projects.restore", err, "Не удалось восстановить проект. Повторите запрос.")
		return
	}
	fresh, err := s.q.GetProject(r.Context(), row.ID)
	if err != nil {
		s.internal(w, r, "projects.restore.reload", err, "Проект восстановлен, но не открылся. Обновите страницу.")
		return
	}
	p, ok := s.projectJSONFromRow(w, r, fresh)
	if !ok {
		return
	}
	p.Access = RoleOwner
	writeProject(w, http.StatusOK, p)
}

func (s *Server) purgeProject(w http.ResponseWriter, r *http.Request) {
	row := trashedFrom(r)
	actor := auth.FromRequest(r).UserID
	err := s.inTx(r.Context(), func(tx *Server) error {
		n, err := tx.q.PurgeProject(r.Context(), row.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return &httpError{status: http.StatusNotFound, msg: "Проект не найден."}
		}
		return tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionProjectPurge, TargetType: audit.TargetProject, TargetID: row.ID.String(),
		})
	})
	if err != nil {
		if e, ok := err.(*httpError); ok {
			writeError(w, r, e.status, e.msg)
			return
		}
		s.internal(w, r, "projects.purge", err, "Не удалось удалить проект окончательно. Повторите запрос.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PurgeTrash deletes projects whose retention ran out, in batches. It returns how many went.
func PurgeTrash(ctx context.Context, q *db.Queries, before time.Time, batch int32) (int, error) {
	ids, err := q.PurgeExpiredProjects(ctx, db.PurgeExpiredProjectsParams{
		Before:  pgtype.Timestamptz{Time: before, Valid: true},
		MaxRows: batch,
	})
	if err != nil {
		return 0, fmt.Errorf("projects.purge_expired: %w", err)
	}
	return len(ids), nil
}
