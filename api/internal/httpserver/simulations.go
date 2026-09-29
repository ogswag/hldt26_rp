package httpserver

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/jobs"
	"moscow_hackathon_2026/api/internal/maps"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/simbuild"
	"moscow_hackathon_2026/api/internal/simjobs"
)

const (
	simMaxAttempts  = 3
	simHistoryLimit = 50
)

// Notifier wakes the simulation worker pool.
type Notifier interface {
	Notify()
}

type simulationBody struct {
	VariantID string `json:"variant_id"`
	LineKey   string `json:"line_key"`
	sim.Config
}

type simulationJobJSON struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	RunID             *string         `json:"run_id"`
	Status            string          `json:"status"`
	ProgressPct       int32           `json:"progress_pct"`
	Attempt           int32           `json:"attempt"`
	MaxAttempts       int32           `json:"max_attempts"`
	ErrorText         *string         `json:"error_text"`
	ReplicationsTotal int32           `json:"replications_total"`
	ReplicationsDone  int32           `json:"replications_done"`
	Config            json.RawMessage `json:"config"`
	CreatedAt         *time.Time      `json:"created_at"`
	UpdatedAt         *time.Time      `json:"updated_at"`
	StartedAt         *time.Time      `json:"started_at"`
	FinishedAt        *time.Time      `json:"finished_at"`
	CanceledAt        *time.Time      `json:"canceled_at"`
}

func jobJSON(j db.SimulationJob) simulationJobJSON {
	return simulationJobJSON{
		ID:                j.ID.String(),
		ProjectID:         j.ProjectID.String(),
		RunID:             uuidStringPtr(j.RunID),
		Status:            j.Status,
		ProgressPct:       j.ProgressPct,
		Attempt:           j.Attempt,
		MaxAttempts:       j.MaxAttempts,
		ErrorText:         j.ErrorText,
		ReplicationsTotal: j.ReplicationsTotal,
		ReplicationsDone:  j.ReplicationsDone,
		Config:            j.Config,
		CreatedAt:         timestamptzPtr(j.CreatedAt),
		UpdatedAt:         timestamptzPtr(j.UpdatedAt),
		StartedAt:         timestamptzPtr(j.StartedAt),
		FinishedAt:        timestamptzPtr(j.FinishedAt),
		CanceledAt:        timestamptzPtr(j.CanceledAt),
	}
}

type simulationPreview struct {
	VariantID    string             `json:"variant_id"`
	VariantName  string             `json:"variant_name"`
	MapSource    string             `json:"map_source"`
	Confidence   string             `json:"confidence_level"`
	Replications int                `json:"replications"`
	ExpectedJobs map[string]float64 `json:"expected_jobs"`
	Warnings     []string           `json:"warnings"`
	MapIssues    []maps.Issue       `json:"map_issues"`
}

type createSimulationJSON struct {
	Job       simulationJobJSON `json:"job"`
	RunID     string            `json:"run_id"`
	VersionNo int               `json:"version_no"`
	InputHash string            `json:"input_hash"`
	Preview   simulationPreview `json:"preview"`
}

func (s *Server) simulationsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.q == nil || s.queue == nil {
		writeError(w, r, http.StatusServiceUnavailable, "Очередь симуляций недоступна. Повторите позже.")
		return false
	}
	return true
}

func writeSimInputError(w http.ResponseWriter, r *http.Request, err error) bool {
	var me *simbuild.MapError
	if errors.As(err, &me) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":      "Карта содержит ошибки, симуляцию запустить нельзя. Исправьте их в редакторе карты.",
			"code":       "map_invalid",
			"request_id": requestID(r),
			"issues":     me.Issues,
		})
		return true
	}
	if msg, ok := simbuild.UserMessage(err); ok {
		writeError(w, r, http.StatusBadRequest, msg)
		return true
	}
	return false
}

func (s *Server) createSimulation(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	if !s.simulationsReady(w, r) {
		return
	}
	var body simulationBody
	if !decodeJSON(w, r, &body, true) {
		return
	}
	if _, err := filledValidated(row.ObjectType, row.Params); err != nil {
		rejectParams(w, r, err)
		return
	}
	cfg, err := body.Config.Normalize()
	if err != nil {
		writeSimInputError(w, r, err)
		return
	}
	ctx := r.Context()
	uid := auth.FromRequest(r).UserID
	active, err := s.q.CountActiveSimulationJobsByUser(ctx, &uid)
	if err != nil {
		s.internal(w, r, "sim.create.quota", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	if active >= int64(s.cfg.SimActivePerUser) {
		writeError(w, r, http.StatusTooManyRequests, "У вас уже "+strconv.FormatInt(active, 10)+" активных симуляций. Дождитесь завершения или отмените одну.")
		return
	}
	d, hash, err := s.ensureProjectGraph(ctx, row)
	if err != nil {
		s.internal(w, r, "sim.create.graph", err, "Не удалось подготовить проект. Повторите запрос.")
		return
	}
	jc := simbuild.JobConfig{VariantID: body.VariantID, Sim: cfg, LineKey: body.LineKey}
	if body.LineKey != "" {
		jc.Mode = simbuild.ModeFleetSearch
	}
	if body.VariantID == "" && !simbuild.HasFleet(d) && row.ObjectType == "warehouse" {
		if jc.Suggestion, err = simbuild.SuggestionFrom(row.Results); err != nil {
			writeSimInputError(w, r, err)
			return
		}
	}
	robots, err := s.robotsFor(ctx, d, jc.Suggestion)
	if err != nil {
		s.internal(w, r, "sim.create.robots", err, "Не удалось подготовить симуляцию. Повторите запрос.")
		return
	}
	built, err := simbuild.Build(robots, row.ObjectType, d, jc)
	if err != nil {
		if !writeSimInputError(w, r, err) {
			s.internal(w, r, "sim.create.build", err, "Не удалось подготовить симуляцию. Повторите запрос.")
		}
		return
	}
	jc.VariantID = built.Variant.ID
	jc.Sim = built.Model.Config()
	rawCfg, err := json.Marshal(jc)
	if err != nil {
		s.internal(w, r, "sim.create.config", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	tx, err := s.q.BeginTx(ctx)
	if err != nil {
		s.internal(w, r, "sim.create.begin", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	model := projects.SnapshotModel{SimVersion: sim.Version, ConfidenceLevel: built.Confidence}
	ver, snap, err := insertVersion(ctx, q, row, d, hash, model)
	if err != nil {
		s.internal(w, r, "sim.create.version", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	busy, err := q.CountActiveSimulationJobsByProject(ctx, row.ID)
	if err != nil {
		s.internal(w, r, "sim.create.busy", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	if busy > 0 {
		writeError(w, r, http.StatusConflict, "Для проекта уже идёт симуляция. Дождитесь завершения или отмените её.")
		return
	}
	run, err := q.InsertSimulationRun(ctx, db.InsertSimulationRunParams{
		ProjectID:        row.ID,
		ProjectVersionID: ver.ID,
		InputHash:        hash,
		MatchVersion:     projects.MatchVersion,
		EconVersion:      econ.ModelVersion,
		SimVersion:       model.SimVersion,
		Seed:             int32(cfg.Seed),
		ConfidenceLevel:  model.ConfidenceLevel,
	})
	if err != nil {
		s.internal(w, r, "sim.create.run", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	repsTotal := int32(built.Model.Replications())
	attempts := int32(simMaxAttempts)
	if jc.Mode == simbuild.ModeFleetSearch {
		attempts = 1
		if q0, ok := simbuild.LineQuantity(built.Variant, jc.LineKey); ok {
			repsTotal = int32(simbuild.SearchStepCap(q0))
		}
	}
	job, err := q.CreateSimulationJob(ctx, db.CreateSimulationJobParams{
		ProjectID:         row.ID,
		RunID:             &run.ID,
		UserID:            &uid,
		Config:            rawCfg,
		ReplicationsTotal: repsTotal,
		MaxAttempts:       attempts,
	})
	if err != nil {
		s.internal(w, r, "sim.create.job", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.internal(w, r, "sim.create.commit", err, "Не удалось поставить симуляцию в очередь. Повторите запрос.")
		return
	}
	s.queue.Notify()
	s.log.Info("sim.enqueued", "op", "sim.create", "request_id", requestID(r), "project_id", row.ID.String(), "job_id", job.ID.String())
	issues := built.MapIssues
	if issues == nil {
		issues = []maps.Issue{}
	}
	writeJSON(w, http.StatusAccepted, createSimulationJSON{
		Job:       jobJSON(job),
		RunID:     run.ID.String(),
		VersionNo: snap.VersionNo,
		InputHash: hash,
		Preview: simulationPreview{
			VariantID:    built.Variant.ID,
			VariantName:  built.Variant.Name,
			MapSource:    built.MapSource,
			Confidence:   built.Confidence,
			Replications: built.Model.Replications(),
			ExpectedJobs: built.Model.ExpectedArrivals(),
			Warnings:     built.AllWarnings(),
			MapIssues:    issues,
		},
	})
}

type simulationListItem struct {
	simulationJobJSON
	InputHash        string          `json:"input_hash"`
	Seed             int32           `json:"seed"`
	SimVersion       string          `json:"sim_version"`
	ProjectVersionID string          `json:"project_version_id"`
	StaleVsDraft     bool            `json:"stale_vs_draft"`
	Brief            json.RawMessage `json:"brief"`
}

func (s *Server) listSimulations(w http.ResponseWriter, r *http.Request) {
	row := projectFrom(r)
	ctx := r.Context()
	d, hash, err := s.ensureProjectGraph(ctx, row)
	if err != nil {
		s.internal(w, r, "sim.list.graph", err, "Не удалось загрузить симуляции. Повторите запрос.")
		return
	}
	rows, err := s.q.ListSimulationJobsByProject(ctx, db.ListSimulationJobsByProjectParams{ProjectID: row.ID, Limit: simHistoryLimit})
	if err != nil {
		s.internal(w, r, "sim.list", err, "Не удалось загрузить симуляции. Повторите запрос.")
		return
	}
	items := make([]simulationListItem, 0, len(rows))
	for _, j := range rows {
		items = append(items, simulationListItem{
			simulationJobJSON: jobJSON(db.SimulationJob{
				ID: j.ID, ProjectID: j.ProjectID, RunID: j.RunID, Status: j.Status, ProgressPct: j.ProgressPct,
				Attempt: j.Attempt, ErrorText: j.ErrorText, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
				CanceledAt: j.CanceledAt, Config: j.Config, ReplicationsTotal: j.ReplicationsTotal,
				ReplicationsDone: j.ReplicationsDone, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, MaxAttempts: simMaxAttempts,
			}),
			InputHash:        j.InputHash,
			Seed:             j.Seed,
			SimVersion:       j.SimVersion,
			ProjectVersionID: j.ProjectVersionID.String(),
			StaleVsDraft:     simStaleFor(d, hash, j.DraftHash, j.SimVersion, j.VariantID, j.VariantHash),
			Brief:            j.Brief,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "input_hash": hash})
}

// simStaleFor reports whether a simulation was made for other inputs than its variant has now. A run that recorded
// the hash of its variant is judged by it, so editing another variant or the costs leaves it current; an older run
// is judged by the whole draft.
func simStaleFor(d projects.Draft, draftHash, runDraftHash, simVersion, variantID, variantHash string) bool {
	if simVersion != sim.Version {
		return true
	}
	// NOTE: the suggestion is not a variant of the draft, so a run on it follows the whole draft.
	if variantID != "" && variantID != projects.SuggestionVariantID && variantHash != "" {
		if cur, err := simbuild.VariantHash(d, variantID); err == nil {
			return cur != variantHash
		}
	}
	return projects.Stale(draftHash, runDraftHash)
}

// summaryVariant reads which variant a stored simulation summary is about and the hash of its inputs.
func summaryVariant(summary []byte) (variantID, variantHash string) {
	var v struct {
		VariantID   string `json:"variant_id"`
		VariantHash string `json:"variant_hash"`
	}
	if json.Unmarshal(summary, &v) != nil {
		return "", ""
	}
	return v.VariantID, v.VariantHash
}

// scopedJob loads the simulation job of a route scoped by {jobId}.
func (s *Server) scopedJob(w http.ResponseWriter, r *http.Request) (db.SimulationJob, db.GetProjectRow, bool) {
	if !s.simulationsReady(w, r) {
		return db.SimulationJob{}, db.GetProjectRow{}, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "jobId"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор задания.")
		return db.SimulationJob{}, db.GetProjectRow{}, false
	}
	job, err := s.q.GetSimulationJob(r.Context(), id)
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusNotFound, "Задание не найдено.")
		} else {
			s.internal(w, r, "sim.job.get", err, "Не удалось загрузить задание. Повторите запрос.")
		}
		return db.SimulationJob{}, db.GetProjectRow{}, false
	}
	return job, projectFrom(r), true
}

func (s *Server) getSimulationJob(w http.ResponseWriter, r *http.Request) {
	job, _, ok := s.scopedJob(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, jobJSON(job))
}

func (s *Server) cancelSimulationJob(w http.ResponseWriter, r *http.Request) {
	job, _, ok := s.scopedJob(w, r)
	if !ok {
		return
	}
	updated, err := s.q.CancelSimulationJob(r.Context(), job.ID)
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusConflict, "Задание уже завершено, отменять нечего.")
			return
		}
		s.internal(w, r, "sim.job.cancel", err, "Не удалось отменить задание. Повторите запрос.")
		return
	}
	if err := s.q.SyncSimulationRunStatus(r.Context(), job.ID); err != nil {
		s.internal(w, r, "sim.job.cancel.sync", err, "Не удалось отменить задание. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusAccepted, jobJSON(updated))
}

func (s *Server) retrySimulationJob(w http.ResponseWriter, r *http.Request) {
	job, proj, ok := s.scopedJob(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	busy, err := s.q.CountActiveSimulationJobsByProject(ctx, proj.ID)
	if err != nil {
		s.internal(w, r, "sim.job.retry.busy", err, "Не удалось повторить задание. Повторите запрос.")
		return
	}
	if busy > 0 {
		writeError(w, r, http.StatusConflict, "Для проекта уже идёт симуляция. Дождитесь завершения или отмените её.")
		return
	}
	updated, err := s.q.RetrySimulationJob(ctx, job.ID)
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusConflict, "Повторить можно только задание с ошибкой или отменённое.")
			return
		}
		s.internal(w, r, "sim.job.retry", err, "Не удалось повторить задание. Повторите запрос.")
		return
	}
	if err := s.q.SyncSimulationRunStatus(ctx, job.ID); err != nil {
		s.internal(w, r, "sim.job.retry.sync", err, "Не удалось повторить задание. Повторите запрос.")
		return
	}
	s.queue.Notify()
	writeJSON(w, http.StatusAccepted, jobJSON(updated))
}

type simulationRunJSON struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	ProjectVersionID string     `json:"project_version_id"`
	InputHash        string     `json:"input_hash"`
	MatchVersion     string     `json:"match_version"`
	EconVersion      string     `json:"econ_version"`
	SimVersion       string     `json:"sim_version"`
	Seed             int32      `json:"seed"`
	Status           string     `json:"status"`
	CreatedAt        *time.Time `json:"created_at"`
	VersionNo        int32      `json:"version_no"`
	ConfidenceLevel  string     `json:"confidence_level"`
}

type artifactJSON struct {
	Kind       string     `json:"kind"`
	Encoding   string     `json:"encoding"`
	SizeBytes  int32      `json:"size_bytes"`
	RawBytes   int32      `json:"raw_bytes"`
	EventCount int32      `json:"event_count"`
	Pinned     bool       `json:"pinned"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  *time.Time `json:"created_at"`
}

type replicationJSON struct {
	Idx     int32           `json:"idx"`
	Seed    int64           `json:"seed"`
	Status  string          `json:"status"`
	Metrics json.RawMessage `json:"metrics"`
}

type simulationRunResponse struct {
	Run          simulationRunJSON  `json:"run"`
	Job          *simulationJobJSON `json:"job"`
	StaleVsDraft bool               `json:"stale_vs_draft"`
	Summary      json.RawMessage    `json:"summary"`
	Replications []replicationJSON  `json:"replications"`
	Artifact     *artifactJSON      `json:"artifact"`
}

// scopedSimRun loads the simulation run of a route scoped by {runId}.
func (s *Server) scopedSimRun(w http.ResponseWriter, r *http.Request) (db.GetSimulationRunRow, db.GetProjectRow, bool) {
	if !s.simulationsReady(w, r) {
		return db.GetSimulationRunRow{}, db.GetProjectRow{}, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Некорректный идентификатор запуска.")
		return db.GetSimulationRunRow{}, db.GetProjectRow{}, false
	}
	run, err := s.q.GetSimulationRun(r.Context(), id)
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusNotFound, "Запуск симуляции не найден.")
		} else {
			s.internal(w, r, "sim.run.get", err, "Не удалось загрузить запуск. Повторите запрос.")
		}
		return db.GetSimulationRunRow{}, db.GetProjectRow{}, false
	}
	return run, projectFrom(r), true
}

func (s *Server) getSimulationRun(w http.ResponseWriter, r *http.Request) {
	run, proj, ok := s.scopedSimRun(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	d, hash, err := s.ensureProjectGraph(ctx, proj)
	if err != nil {
		s.internal(w, r, "sim.run.graph", err, "Не удалось загрузить запуск. Повторите запрос.")
		return
	}
	variantID, variantHash := summaryVariant(run.Summary)
	resp := simulationRunResponse{
		Run: simulationRunJSON{
			ID: run.ID.String(), ProjectID: run.ProjectID.String(), ProjectVersionID: run.ProjectVersionID.String(),
			InputHash: run.InputHash, MatchVersion: run.MatchVersion, EconVersion: run.EconVersion,
			SimVersion: run.SimVersion, Seed: run.Seed, Status: run.Status, CreatedAt: timestamptzPtr(run.CreatedAt),
			VersionNo: run.VersionNo, ConfidenceLevel: run.ConfidenceLevel,
		},
		StaleVsDraft: simStaleFor(d, hash, run.DraftHash, run.SimVersion, variantID, variantHash),
		Replications: []replicationJSON{},
	}
	if len(run.Summary) > 0 {
		resp.Summary = run.Summary
	}
	job, err := s.q.GetSimulationJobByRun(ctx, &run.ID)
	if err != nil && !isNoRows(err) {
		s.internal(w, r, "sim.run.job", err, "Не удалось загрузить запуск. Повторите запрос.")
		return
	}
	if err == nil {
		j := jobJSON(job)
		resp.Job = &j
	}
	reps, err := s.q.ListSimulationReplications(ctx, run.ID)
	if err != nil {
		s.internal(w, r, "sim.run.replications", err, "Не удалось загрузить запуск. Повторите запрос.")
		return
	}
	for _, rep := range reps {
		resp.Replications = append(resp.Replications, replicationJSON{Idx: rep.Idx, Seed: rep.Seed, Status: rep.Status, Metrics: rep.Metrics})
	}
	meta, err := s.q.GetSimulationArtifactMeta(ctx, db.GetSimulationArtifactMetaParams{RunID: run.ID, Kind: simjobs.ArtifactEventLog})
	if err != nil && !isNoRows(err) {
		s.internal(w, r, "sim.run.artifact", err, "Не удалось загрузить запуск. Повторите запрос.")
		return
	}
	if err == nil {
		resp.Artifact = &artifactJSON{
			Kind: meta.Kind, Encoding: meta.Encoding, SizeBytes: meta.SizeBytes, RawBytes: meta.RawBytes,
			EventCount: meta.EventCount, Pinned: meta.Pinned, ExpiresAt: timestamptzPtr(meta.ExpiresAt),
			CreatedAt: timestamptzPtr(meta.CreatedAt),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) getSimulationEvents(w http.ResponseWriter, r *http.Request) {
	run, _, ok := s.scopedSimRun(w, r)
	if !ok {
		return
	}
	art, err := s.q.GetSimulationArtifact(r.Context(), db.GetSimulationArtifactParams{RunID: run.ID, Kind: simjobs.ArtifactEventLog})
	if err != nil {
		if isNoRows(err) {
			if run.Status == jobs.StatusSucceeded {
				writeError(w, r, http.StatusGone, "Журнал событий удалён по сроку хранения. Запустите симуляцию заново.")
				return
			}
			writeError(w, r, http.StatusNotFound, "Журнал событий появится после завершения симуляции.")
			return
		}
		s.internal(w, r, "sim.events", err, "Не удалось загрузить журнал. Повторите запрос.")
		return
	}
	if art.Encoding != simjobs.EncodingGzipJSON {
		s.internal(w, r, "sim.events.encoding", errors.New("unknown encoding "+art.Encoding), "Не удалось прочитать журнал. Повторите запрос.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("Vary", "Accept-Encoding")
	if acceptsGzip(r) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(art.Data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(art.Data)
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(art.Data))
	if err != nil {
		s.internal(w, r, "sim.events.gunzip", err, "Не удалось прочитать журнал. Повторите запрос.")
		return
	}
	defer zr.Close()
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, zr)
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		fields := strings.Split(part, ";")
		enc := strings.TrimSpace(fields[0])
		if enc != "gzip" && enc != "*" {
			continue
		}
		for _, f := range fields[1:] {
			f = strings.TrimSpace(f)
			if q, ok := strings.CutPrefix(f, "q="); ok {
				if v, err := strconv.ParseFloat(q, 64); err == nil && v == 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}

type pinBody struct {
	Pinned bool `json:"pinned"`
}

func (s *Server) pinSimulationRun(w http.ResponseWriter, r *http.Request) {
	run, _, ok := s.scopedSimRun(w, r)
	if !ok {
		return
	}
	var body pinBody
	if !decodeJSON(w, r, &body, false) {
		return
	}
	expires := pgtype.Timestamptz{Time: time.Now().Add(s.sims.Retention()), Valid: true}
	res, err := s.q.SetSimulationArtifactPinned(r.Context(), db.SetSimulationArtifactPinnedParams{
		Pinned: body.Pinned, ExpiresAt: expires, RunID: run.ID, Kind: simjobs.ArtifactEventLog,
	})
	if err != nil {
		if isNoRows(err) {
			writeError(w, r, http.StatusNotFound, "У запуска нет журнала событий.")
			return
		}
		s.internal(w, r, "sim.pin", err, "Не удалось изменить хранение журнала. Повторите запрос.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pinned": res.Pinned, "expires_at": timestamptzPtr(res.ExpiresAt)})
}
