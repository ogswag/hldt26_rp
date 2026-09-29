package httpserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/exporters"
	"moscow_hackathon_2026/api/internal/projects"
)

func parseExportFormat(w http.ResponseWriter, r *http.Request) (string, bool) {
	f := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if f == "" {
		f = "pdf"
	}
	if f != "pdf" && f != "xlsx" {
		writeError(w, r, http.StatusBadRequest, "format должен быть pdf или xlsx.")
		return "", false
	}
	return f, true
}

func (s *Server) guestExport(w http.ResponseWriter, r *http.Request) {
	format, ok := parseExportFormat(w, r)
	if !ok {
		return
	}
	var body guestCalculateBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	in, ok := s.readGuestInput(w, r, body)
	if !ok {
		return
	}
	result, err := s.guestResult(r.Context(), in)
	if err != nil {
		s.failCalc(w, r, "guest.export", err)
		return
	}
	inputs := projects.Draft{ObjectType: in.ObjectType, Params: in.Params}
	if in.HasDraft {
		inputs = in.Draft
	}
	rep := exporters.Report{
		GeneratedAt: time.Now().UTC(),
		ObjectType:  in.ObjectType,
		Inputs:      inputs,
		Econ:        &result,
	}
	rep.Catalog, err = s.reportCatalog(r.Context(), append(guestSolutions(result), solutionIDs(inputs.Variants)...))
	if err != nil {
		s.internal(w, r, "guest.export.catalog", err, "Не удалось сформировать файл. Повторите запрос.")
		return
	}
	s.writeReport(w, r, format, rep)
}

func guestSolutions(res econ.Result) []uuid.UUID {
	var out []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, sc := range res.Scenarios {
		if sc.SolutionID == nil {
			continue
		}
		if id, err := uuid.Parse(*sc.SolutionID); err == nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// projectExport renders the current calculation run. A stale run must be exported by its run id.
func (s *Server) projectExport(w http.ResponseWriter, r *http.Request) {
	format, ok := parseExportFormat(w, r)
	if !ok {
		return
	}
	row := projectFrom(r)
	if row.CurrentRunID == nil {
		if len(row.Results) > 0 {
			writeError(w, r, http.StatusConflict, "Результат сохранён до появления запусков и не связан со снимком входов. Пересчитайте проект, затем скачайте отчёт.")
			return
		}
		writeError(w, r, http.StatusBadRequest, "Сначала выполните расчёт. Затем скачайте отчёт.")
		return
	}
	run, err := s.q.GetCalculationRun(r.Context(), *row.CurrentRunID)
	if err != nil {
		s.internal(w, r, "projects.export.run", err, "Не удалось загрузить сохранённый запуск. Повторите запрос.")
		return
	}
	if projects.Stale(row.InputHash, run.DraftHash) {
		writeJSON(w, http.StatusConflict, errorBody{
			Error:     "Черновик изменился после последнего расчёта. Пересчитайте проект или скачайте отчёт исторического запуска в истории.",
			Code:      "stale_result",
			RequestID: requestID(r),
		})
		return
	}
	s.writeRunReport(w, r, format, row, calcRun(run))
}

func (s *Server) writeReport(w http.ResponseWriter, r *http.Request, format string, rep exporters.Report) {
	ctype, ext := contentType(format)
	var (
		data []byte
		err  error
	)
	if ext == "xlsx" {
		data, err = exporters.XLSX(rep)
	} else {
		data, err = exporters.PDF(rep)
	}
	if err != nil {
		s.internal(w, r, "export.build", err, "Не удалось сформировать файл. Повторите запрос.")
		return
	}
	kind, runID := exportLogName(rep)
	s.log.Info("export", "op", "export", "request_id", requestID(r), "kind", kind, "run_id", runID, "format", ext, "bytes", len(data))
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+exporters.Filename(rep, ext)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	if runID != "" {
		w.Header().Set("X-Run-Id", runID)
		w.Header().Set("X-Input-Hash", rep.Run.InputHash)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
