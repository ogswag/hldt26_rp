// Package simjobs runs queued sim-v2 jobs: it reads the project snapshot and the catalog, builds the run with
// simbuild and stores its results.
package simjobs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/catalogstore"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/jobs"
	"moscow_hackathon_2026/api/internal/projects"
	"moscow_hackathon_2026/api/internal/sim"
	"moscow_hackathon_2026/api/internal/simbuild"
)

const (
	ArtifactEventLog = "event_log"
	EncodingGzipJSON = "gzip+json"
)

type Service struct {
	q         *db.Queries
	log       *slog.Logger
	retention time.Duration
}

func New(q *db.Queries, log *slog.Logger, retention time.Duration) *Service {
	if retention <= 0 {
		retention = 90 * 24 * time.Hour
	}
	return &Service{q: q, log: log, retention: retention}
}

// Retention is how long an unpinned journal is kept.
func (s *Service) Retention() time.Duration {
	return s.retention
}

// Run implements jobs.Handler.
func (s *Service) Run(ctx context.Context, job db.SimulationJob, progress *jobs.Progress) error {
	if job.RunID == nil {
		return jobs.Permanent("Задание не связано с запуском. Создайте симуляцию заново.", nil)
	}
	run, err := s.q.GetSimulationRun(ctx, *job.RunID)
	if err != nil {
		return fmt.Errorf("simjobs.run: %w", err)
	}
	ver, err := s.q.GetProjectVersion(ctx, db.GetProjectVersionParams{ID: run.ProjectVersionID, ProjectID: run.ProjectID})
	if err != nil {
		return fmt.Errorf("simjobs.version: %w", err)
	}
	var snap projects.Snapshot
	if err := json.Unmarshal(ver.Snapshot, &snap); err != nil {
		return jobs.Permanent("Снимок проекта повреждён. Создайте симуляцию заново.", err)
	}
	var cfg simbuild.JobConfig
	if err := json.Unmarshal(job.Config, &cfg); err != nil {
		return jobs.Permanent("Параметры симуляции повреждены. Создайте симуляцию заново.", err)
	}
	start := time.Now()
	cat, err := catalogstore.Load(ctx, s.q)
	if err != nil {
		return err
	}
	robots := simbuild.RobotsOf(cat)
	var built *simbuild.Built
	var res sim.Result
	var reps []sim.Metrics
	var journal *sim.Log
	var search simbuild.FleetSearch
	if cfg.Mode == simbuild.ModeFleetSearch {
		deadline := time.Time{}
		if t, ok := ctx.Deadline(); ok {
			left := t.Sub(start)
			deadline = start.Add(time.Duration(float64(left) * 0.7))
		}
		built, res, reps, journal, search, err = simbuild.SearchFleet(ctx, robots, snap.ObjectType, snap.Draft, cfg, func(done float64) error {
			progress.Set(int(math.Min(99, done)), int(done))
			return nil
		}, deadline)
	} else {
		built, err = simbuild.Build(robots, snap.ObjectType, snap.Draft, cfg)
		if err == nil {
			total := built.Model.Replications()
			res, reps, journal, err = built.Model.RunAll(ctx, func(done float64) error {
				progress.Set(int(math.Floor(done/float64(total)*100)), int(done))
				return nil
			})
		}
	}
	if err != nil {
		if msg, ok := simbuild.UserMessage(err); ok {
			return jobs.Permanent(msg, err)
		}
		if errors.Is(err, sim.ErrTooManyEvents) {
			return jobs.Permanent("Модель слишком большая для одного прогона. Сократите горизонт или спрос.", err)
		}
		return err
	}
	summary := simbuild.NewSummary(built, res, simbuild.SummaryMeta{
		RunID:                run.ID.String(),
		ProjectID:            run.ProjectID.String(),
		VersionNo:            snap.VersionNo,
		InputHash:            run.InputHash,
		CatalogContentSHA256: cat.ContentSHA256,
		Snapshot:             simbuild.SnapshotInfo{MatchVersion: run.MatchVersion, EconVersion: run.EconVersion, SimVersion: run.SimVersion},
	}, start, time.Now())
	if cfg.Mode == simbuild.ModeFleetSearch {
		summary.FleetSearch = &search
	}
	rawSummary, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("simjobs.summary: %w", err)
	}
	logBytes, rawLen, err := compress(journal)
	if err != nil {
		return fmt.Errorf("simjobs.log: %w", err)
	}
	tx, err := s.q.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("simjobs.begin: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	qtx := s.q.WithTx(tx)
	if err := qtx.DeleteSimulationReplications(ctx, run.ID); err != nil {
		return fmt.Errorf("simjobs.replications.reset: %w", err)
	}
	for _, r := range reps {
		r.Robots = nil
		raw, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("simjobs.replication: %w", err)
		}
		if err := qtx.UpsertSimulationReplication(ctx, db.UpsertSimulationReplicationParams{
			RunID: run.ID, Idx: int32(r.Replication), Seed: r.Seed, Status: jobs.StatusSucceeded, Metrics: raw,
		}); err != nil {
			return fmt.Errorf("simjobs.replication.save: %w", err)
		}
	}
	if err := qtx.UpsertSimulationArtifact(ctx, db.UpsertSimulationArtifactParams{
		RunID:      run.ID,
		Kind:       ArtifactEventLog,
		Encoding:   EncodingGzipJSON,
		Data:       logBytes,
		SizeBytes:  int32(len(logBytes)),
		RawBytes:   int32(rawLen),
		EventCount: int32(len(journal.Events)),
		ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(s.retention), Valid: true},
	}); err != nil {
		return fmt.Errorf("simjobs.artifact: %w", err)
	}
	if _, err := qtx.InsertCalculationResult(ctx, db.InsertCalculationResultParams{RunID: run.ID, Summary: rawSummary}); err != nil {
		return fmt.Errorf("simjobs.result: %w", err)
	}
	if err := jobs.Complete(ctx, qtx, job.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("simjobs.commit: %w", err)
	}
	s.log.Info("simjobs.done", "op", "simjobs.run", "job_id", job.ID.String(), "project_id", run.ProjectID.String(),
		"replications", len(reps), "events", len(journal.Events), "log_bytes", len(logBytes), "verdict", res.Verdict)
	return nil
}

// Maintain deletes expired journals.
func (s *Service) Maintain(ctx context.Context) error {
	n, err := s.q.DeleteExpiredSimulationArtifacts(ctx)
	if err != nil {
		return fmt.Errorf("simjobs.retention: %w", err)
	}
	if n > 0 {
		s.log.Info("simjobs.retention", "op", "simjobs.retention", "deleted", n)
	}
	return nil
}

func compress(log *sim.Log) ([]byte, int, error) {
	raw, err := json.Marshal(log)
	if err != nil {
		return nil, 0, err
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, 0, err
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, 0, err
	}
	if err := zw.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), len(raw), nil
}
