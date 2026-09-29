package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"moscow_hackathon_2026/api/internal/audit"
	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/internal/objects"
)

var errSolutionNotFound = errors.New("solution not found")

// solutionUpdate is what a save returns: the robot as stored and the fields that changed, for undo.
type solutionUpdate struct {
	Solution solutionJSON       `json:"solution"`
	Changes  []importers.Change `json:"changes"`
}

func writeFieldError(w http.ResponseWriter, r *http.Request, fe *importers.FieldError) {
	writeJSON(w, http.StatusBadRequest, errorBody{
		Error:     fe.Message,
		Code:      "validation_error",
		RequestID: requestID(r),
		Details:   []objects.FieldError{{Field: fe.Field, Message: fe.Message}},
	})
}

func writeSaveError(w http.ResponseWriter, r *http.Request, err error) bool {
	var fe *importers.FieldError
	if errors.As(err, &fe) {
		writeFieldError(w, r, fe)
		return true
	}
	return false
}

// readFieldPatch reads a map of field codes to values in stored units; null clears a field.
func readFieldPatch(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	var body map[string]json.RawMessage
	if !decodeJSON(w, r, &body, false) {
		return nil, false
	}
	patch := make(map[string]any, len(body))
	for code, raw := range body {
		f, ok := importers.FieldByCode(code)
		if !ok || !f.Editable {
			writeError(w, r, http.StatusBadRequest, "В запросе есть поле, которое нельзя изменить. Обновите страницу и повторите.")
			return nil, false
		}
		v, err := f.FromJSON(raw)
		if err != nil {
			if !writeSaveError(w, r, err) {
				writeError(w, r, http.StatusBadRequest, "Значение не прошло проверку. Исправьте его и повторите.")
			}
			return nil, false
		}
		patch[code] = v
	}
	return patch, true
}

func applyPatch(before importers.Values, patch map[string]any) importers.Values {
	after := maps.Clone(before)
	for code, v := range patch {
		if v == nil {
			delete(after, code)
		} else {
			after[code] = v
		}
	}
	return after
}

func (s *Server) solutionIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "solutionId"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор решения.")
		return uuid.Nil, false
	}
	return id, true
}

// lockedSolution locks the robot row for the rest of the transaction and reads it.
func lockedSolution(r *http.Request, q *db.Queries, id uuid.UUID) (db.ListSolutionsRow, error) {
	if _, err := q.LockSolution(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ListSolutionsRow{}, errSolutionNotFound
		}
		return db.ListSolutionsRow{}, err
	}
	row, err := q.GetSolution(r.Context(), id)
	if err != nil {
		return db.ListSolutionsRow{}, err
	}
	return db.ListSolutionsRow(row), nil
}

func (s *Server) solutionFailed(w http.ResponseWriter, r *http.Request, op string, err error, msg string) {
	if errors.Is(err, errSolutionNotFound) {
		writeError(w, r, http.StatusNotFound, "Решение не найдено. Обновите список.")
		return
	}
	if writeSaveError(w, r, err) {
		return
	}
	s.internal(w, r, op, err, msg)
}

// saveSolution applies patch to the locked robot, records the changed fields and returns them.
func (s *Server) saveSolution(r *http.Request, row db.ListSolutionsRow, patch map[string]any, action string) ([]importers.Change, error) {
	before := importers.RowValues(row)
	after := applyPatch(before, patch)
	if fe := importers.Check(after); fe != nil {
		return nil, fe
	}
	changes := importers.Diff(before, after)
	if len(changes) == 0 {
		return nil, nil
	}
	if err := importers.Save(r.Context(), s.q, row, after); err != nil {
		return nil, err
	}
	actor := auth.FromRequest(r).UserID
	return changes, s.recordAudit(r.Context(), requestID(r), audit.Event{
		Actor:      &actor,
		Action:     action,
		TargetType: audit.TargetSolution,
		TargetID:   row.ID.String(),
		Meta:       map[string]any{"name": row.Name, "changes": changes},
	})
}

func (s *Server) adminCreateSolution(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	patch, ok := readFieldPatch(w, r)
	if !ok {
		return
	}
	if fe := importers.Check(applyPatch(importers.Values{}, patch)); fe != nil {
		writeFieldError(w, r, fe)
		return
	}
	id := uuid.New()
	var out solutionUpdate
	err := s.inTx(r.Context(), func(tx *Server) error {
		name, _ := patch["name"].(string)
		if err := tx.q.CreateSolution(r.Context(), db.CreateSolutionParams{ID: id, Name: name, Raw: json.RawMessage(`{}`)}); err != nil {
			return err
		}
		if err := tx.q.InsertSolutionSpecs(r.Context(), id); err != nil {
			return err
		}
		row, err := lockedSolution(r, tx.q, id)
		if err != nil {
			return err
		}
		if out.Changes, err = tx.saveSolution(r, row, patch, audit.ActionSolutionCreate); err != nil {
			return err
		}
		out.Solution, err = tx.solutionByID(r, id)
		return err
	})
	if err != nil {
		s.solutionFailed(w, r, "admin.solution.create", err, "Не удалось создать решение. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) adminPatchSolution(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := s.solutionIDParam(w, r)
	if !ok {
		return
	}
	patch, ok := readFieldPatch(w, r)
	if !ok {
		return
	}
	var out solutionUpdate
	err := s.inTx(r.Context(), func(tx *Server) error {
		row, err := lockedSolution(r, tx.q, id)
		if err != nil {
			return err
		}
		if out.Changes, err = tx.saveSolution(r, row, patch, audit.ActionSolutionUpdate); err != nil {
			return err
		}
		out.Solution, err = tx.solutionByID(r, id)
		return err
	})
	if err != nil {
		s.solutionFailed(w, r, "admin.solution.patch", err, "Не удалось сохранить поле. Повторите.")
		return
	}
	if out.Changes == nil {
		out.Changes = []importers.Change{}
	}
	writeJSON(w, http.StatusOK, out)
}

// legacyFields are the fields the full-replace PUT body carries; the others keep their values.
var legacyFields = []string{
	"name", "vendor", "kind", "subtype", "status", "industry", "scenario", "price_rub", "source_url",
	"payload_kg", "mass_kg", "length_mm", "width_mm", "height_mm", "speed_mps", "endurance_h", "charge_min",
	"nav_type", "pos_accuracy_mm", "min_aisle_mm", "temp_min_c", "temp_max_c", "lifetime_years",
	"service_pct_year", "confidence", "sourced_at",
}

// adminUpdateSolution replaces the fields of the older full form in one request.
func (s *Server) adminUpdateSolution(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := s.solutionIDParam(w, r)
	if !ok {
		return
	}
	var body struct {
		Specs map[string]json.RawMessage `json:"specs"`
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || json.Unmarshal(raw, &body) != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный JSON. Проверьте тело запроса.")
		return
	}
	var top map[string]json.RawMessage
	_ = json.Unmarshal(raw, &top)
	patch := make(map[string]any, len(legacyFields))
	for _, code := range legacyFields {
		v, found := top[code]
		if !found {
			v = body.Specs[code]
		}
		f, _ := importers.FieldByCode(code)
		x, err := f.FromJSON(v)
		if err != nil {
			writeSaveError(w, r, err)
			return
		}
		patch[code] = x
	}
	var out solutionJSON
	err = s.inTx(r.Context(), func(tx *Server) error {
		row, err := lockedSolution(r, tx.q, id)
		if err != nil {
			return err
		}
		if _, err := tx.saveSolution(r, row, patch, audit.ActionSolutionUpdate); err != nil {
			return err
		}
		out, err = tx.solutionByID(r, id)
		return err
	})
	if err != nil {
		s.solutionFailed(w, r, "admin.solution.update", err, "Не удалось сохранить решение. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// adminDeleteSolution moves a robot to the archive: it leaves the lists and matching, fleets keep it.
func (s *Server) adminDeleteSolution(w http.ResponseWriter, r *http.Request) {
	s.setArchived(w, r, true)
}

func (s *Server) adminRestoreSolution(w http.ResponseWriter, r *http.Request) {
	s.setArchived(w, r, false)
}

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, archive bool) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := s.solutionIDParam(w, r)
	if !ok {
		return
	}
	var out solutionJSON
	err := s.inTx(r.Context(), func(tx *Server) error {
		row, err := lockedSolution(r, tx.q, id)
		if err != nil {
			return err
		}
		action, n := audit.ActionSolutionRestore, int64(0)
		if archive {
			action = audit.ActionSolutionArchive
			n, err = tx.q.ArchiveSolution(r.Context(), id)
		} else {
			n, err = tx.q.RestoreSolution(r.Context(), id)
		}
		if err != nil {
			return err
		}
		if n > 0 {
			actor := auth.FromRequest(r).UserID
			if err := tx.recordAudit(r.Context(), requestID(r), audit.Event{
				Actor: &actor, Action: action, TargetType: audit.TargetSolution, TargetID: id.String(),
				Meta: map[string]any{"name": row.Name},
			}); err != nil {
				return err
			}
		}
		out, err = tx.solutionByID(r, id)
		return err
	})
	if err != nil {
		s.solutionFailed(w, r, "admin.solution.archive", err, "Не удалось изменить архив. Повторите запрос.")
		return
	}
	if archive {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// copyName names a duplicate so it fits the name limit.
func copyName(name string) string {
	const suffix = " (копия)"
	f, _ := importers.FieldByCode("name")
	limit := f.MaxLen - utf8.RuneCountInString(suffix)
	if r := []rune(name); len(r) > limit {
		name = strings.TrimSpace(string(r[:limit]))
	}
	return name + suffix
}

func (s *Server) adminDuplicateSolution(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := s.solutionIDParam(w, r)
	if !ok {
		return
	}
	newID := uuid.New()
	var out solutionJSON
	err := s.inTx(r.Context(), func(tx *Server) error {
		src, err := lockedSolution(r, tx.q, id)
		if err != nil {
			return err
		}
		if err := tx.q.CreateSolution(r.Context(), db.CreateSolutionParams{
			ID: newID, Name: copyName(src.Name), Vendor: src.Vendor, Kind: src.Kind, Subtype: src.Subtype,
			Status: src.Status, Industry: src.Industry, Scenario: src.Scenario, PriceRub: src.PriceRub,
			SourceUrl: src.SourceUrl, Raw: src.Raw, Family: src.Family,
		}); err != nil {
			return err
		}
		specs, err := importers.SpecsParams(newID, importers.RowValues(src))
		if err != nil {
			return err
		}
		if err := tx.q.UpsertSolutionSpecs(r.Context(), specs); err != nil {
			return err
		}
		if err := tx.q.CopySolutionImage(r.Context(), db.CopySolutionImageParams{ToID: newID, FromID: id}); err != nil {
			return err
		}
		actor := auth.FromRequest(r).UserID
		if err := tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionSolutionDuplicate, TargetType: audit.TargetSolution,
			TargetID: newID.String(), Meta: map[string]any{"name": copyName(src.Name), "source_id": id.String()},
		}); err != nil {
			return err
		}
		out, err = tx.solutionByID(r, newID)
		return err
	})
	if err != nil {
		s.solutionFailed(w, r, "admin.solution.duplicate", err, "Не удалось создать копию. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) adminPutSolutionImage(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := s.solutionIDParam(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, importers.MaxPhotoBytes))
	if err != nil {
		writeError(w, r, http.StatusRequestEntityTooLarge, "Фото больше 8 МБ. Уменьшите его и загрузите снова.")
		return
	}
	photo, err := importers.NormalizePhoto(data)
	if err != nil {
		switch {
		case errors.Is(err, importers.ErrPhotoFormat):
			writeError(w, r, http.StatusBadRequest, "Файл не похож на фото. Подходят JPEG, PNG, WebP и GIF.")
		case errors.Is(err, importers.ErrPhotoPixels):
			writeError(w, r, http.StatusBadRequest, "Фото больше 40 мегапикселей. Уменьшите его и загрузите снова.")
		default:
			s.internal(w, r, "admin.solution.image", err, "Не удалось обработать фото. Повторите.")
		}
		return
	}
	var out solutionJSON
	err = s.inTx(r.Context(), func(tx *Server) error {
		row, err := lockedSolution(r, tx.q, id)
		if err != nil {
			return err
		}
		if err := tx.q.UpsertSolutionImage(r.Context(), db.UpsertSolutionImageParams{
			SolutionID: id, ContentType: photo.ContentType, Bytes: photo.Bytes, Sha256: photo.SHA256,
		}); err != nil {
			return err
		}
		actor := auth.FromRequest(r).UserID
		if err := tx.recordAudit(r.Context(), requestID(r), audit.Event{
			Actor: &actor, Action: audit.ActionSolutionImage, TargetType: audit.TargetSolution, TargetID: id.String(),
			Meta: map[string]any{"name": row.Name, "image_sha": photo.SHA256},
		}); err != nil {
			return err
		}
		out, err = tx.solutionByID(r, id)
		return err
	})
	if err != nil {
		s.solutionFailed(w, r, "admin.solution.image", err, "Не удалось сохранить фото. Повторите.")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminDeleteSolutionImage(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, ok := s.solutionIDParam(w, r)
	if !ok {
		return
	}
	var out solutionJSON
	err := s.inTx(r.Context(), func(tx *Server) error {
		row, err := lockedSolution(r, tx.q, id)
		if err != nil {
			return err
		}
		n, err := tx.q.DeleteSolutionImage(r.Context(), id)
		if err != nil {
			return err
		}
		if n > 0 {
			actor := auth.FromRequest(r).UserID
			if err := tx.recordAudit(r.Context(), requestID(r), audit.Event{
				Actor: &actor, Action: audit.ActionSolutionImage, TargetType: audit.TargetSolution, TargetID: id.String(),
				Meta: map[string]any{"name": row.Name, "image_sha": nil},
			}); err != nil {
				return err
			}
		}
		out, err = tx.solutionByID(r, id)
		return err
	})
	if err != nil {
		s.solutionFailed(w, r, "admin.solution.image", err, "Не удалось удалить фото. Повторите.")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// solutionImage serves a robot's photo to everyone. Its URL carries the photo's hash, so browsers keep it.
func (s *Server) solutionImage(w http.ResponseWriter, r *http.Request) {
	if s.q == nil {
		writeError(w, r, http.StatusNotFound, "Фото нет.")
		return
	}
	id, ok := s.solutionIDParam(w, r)
	if !ok {
		return
	}
	img, err := s.q.GetSolutionImage(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "У этого решения нет фото.")
			return
		}
		s.internal(w, r, "catalog.image", err, "Не удалось загрузить фото. Повторите запрос.")
		return
	}
	etag := `"` + img.Sha256 + `"`
	w.Header().Set("ETag", etag)
	if r.URL.Query().Get("v") == img.Sha256 {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img.Bytes)
}

func (s *Server) solutionByID(r *http.Request, id uuid.UUID) (solutionJSON, error) {
	row, err := s.q.GetSolution(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return solutionJSON{}, errSolutionNotFound
		}
		return solutionJSON{}, err
	}
	return solutionJSONFromRow(db.ListSolutionsRow(row)), nil
}
