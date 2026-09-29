package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/testdb"
)

type fakeHandler struct {
	q     *db.Queries
	mode  atomic.Value
	calls atomic.Int32
}

func (h *fakeHandler) Run(ctx context.Context, job db.SimulationJob, progress *Progress) error {
	h.calls.Add(1)
	progress.Set(40, 2)
	switch h.mode.Load().(string) {
	case "block":
		<-ctx.Done()
		return ctx.Err()
	case "transient":
		return errors.New("database hiccup")
	case "permanent":
		return Permanent("Карта содержит ошибки.", nil)
	default:
		tx, err := h.q.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := Complete(ctx, h.q.WithTx(tx), job.ID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
}

func seedJob(t *testing.T, pool *pgxpool.Pool, q *db.Queries) db.SimulationJob {
	t.Helper()
	ctx := context.Background()
	var projectID, versionID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO projects (name, object_type, model_version) VALUES ('p', 'warehouse', 'econ-v1') RETURNING id`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO project_versions (project_id, version_no, snapshot, input_hash) VALUES ($1, 1, '{}', 'h') RETURNING id`, projectID).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	run, err := q.InsertSimulationRun(ctx, db.InsertSimulationRunParams{
		ProjectID: projectID, ProjectVersionID: versionID, InputHash: "h", MatchVersion: "m", EconVersion: "e", SimVersion: "sim-v2", ConfidenceLevel: "preliminary",
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := q.CreateSimulationJob(ctx, db.CreateSimulationJobParams{
		ProjectID: projectID, RunID: &run.ID, Config: []byte(`{}`), ReplicationsTotal: 5, MaxAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func waitStatus(t *testing.T, q *db.Queries, id uuid.UUID, want string) db.SimulationJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		job, err := q.GetSimulationJob(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == want {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job status %s, want %s (%v)", job.Status, want, job.ErrorText)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func runStatus(t *testing.T, pool *pgxpool.Pool, job db.SimulationJob) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM calculation_runs WHERE id = $1`, *job.RunID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPoolLifecycle(t *testing.T) {
	pool, q := testdb.New(t, "../../../data")
	h := &fakeHandler{q: q}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := Config{Workers: 1, Poll: 20 * time.Millisecond, Heartbeat: 30 * time.Millisecond, StaleAfter: time.Hour, JanitorEvery: time.Hour}

	t.Run("cancel while running", func(t *testing.T) {
		h.mode.Store("block")
		p := New(q, h, log, cfg)
		stop := p.Start(context.Background())
		defer stop()
		job := seedJob(t, pool, q)
		waitStatus(t, q, job.ID, StatusRunning)
		if runStatus(t, pool, job) != StatusRunning {
			t.Fatal("run status not synced on claim")
		}
		if _, err := q.CancelSimulationJob(context.Background(), job.ID); err != nil {
			t.Fatal(err)
		}
		got := waitStatus(t, q, job.ID, StatusCanceled)
		if got.ProgressPct != 40 || got.ReplicationsDone != 2 || !got.FinishedAt.Valid {
			t.Fatalf("canceled job %+v", got)
		}
		if runStatus(t, pool, job) != StatusCanceled {
			t.Fatal("run status not synced on cancel")
		}
	})

	t.Run("transient error retries then fails", func(t *testing.T) {
		h.mode.Store("transient")
		h.calls.Store(0)
		p := New(q, h, log, cfg)
		stop := p.Start(context.Background())
		defer stop()
		job := seedJob(t, pool, q)
		got := waitStatus(t, q, job.ID, StatusFailed)
		if h.calls.Load() != 2 || got.Attempt != 2 || got.ErrorText == nil {
			t.Fatalf("calls %d job %+v", h.calls.Load(), got)
		}
	})

	t.Run("permanent error keeps the message", func(t *testing.T) {
		h.mode.Store("permanent")
		h.calls.Store(0)
		p := New(q, h, log, cfg)
		stop := p.Start(context.Background())
		defer stop()
		job := seedJob(t, pool, q)
		got := waitStatus(t, q, job.ID, StatusFailed)
		if h.calls.Load() != 1 || got.ErrorText == nil || *got.ErrorText != "Карта содержит ошибки." {
			t.Fatalf("calls %d job %+v", h.calls.Load(), got)
		}
		retried, err := q.RetrySimulationJob(context.Background(), job.ID)
		if err != nil || retried.Status != StatusQueued || retried.Attempt != 0 {
			t.Fatalf("retry %+v %v", retried, err)
		}
		h.mode.Store("ok")
		done := waitStatus(t, q, job.ID, StatusSucceeded)
		if done.ProgressPct != 100 || done.ReplicationsDone != 5 || runStatus(t, pool, job) != StatusSucceeded {
			t.Fatalf("done %+v", done)
		}
	})

	t.Run("shutdown requeues without spending an attempt", func(t *testing.T) {
		h.mode.Store("block")
		p := New(q, h, log, cfg)
		stop := p.Start(context.Background())
		job := seedJob(t, pool, q)
		waitStatus(t, q, job.ID, StatusRunning)
		stop()
		got := waitStatus(t, q, job.ID, StatusQueued)
		if got.Attempt != 0 || runStatus(t, pool, job) != StatusQueued {
			t.Fatalf("requeued %+v", got)
		}
		if _, err := q.CancelSimulationJob(context.Background(), job.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("janitor recovers a job with a dead worker", func(t *testing.T) {
		job := seedJob(t, pool, q)
		claimed, err := q.ClaimSimulationJob(context.Background())
		if err != nil || claimed.ID != job.ID {
			t.Fatalf("claim %v %v", claimed.ID, err)
		}
		if _, err := pool.Exec(context.Background(), `UPDATE simulation_jobs SET heartbeat_at = now() - interval '1 hour' WHERE id = $1`, job.ID); err != nil {
			t.Fatal(err)
		}
		p := New(q, h, log, Config{StaleAfter: time.Minute})
		p.Recover(context.Background())
		got := waitStatus(t, q, job.ID, StatusQueued)
		if got.Attempt != 1 {
			t.Fatalf("attempt %d", got.Attempt)
		}
		if _, err := pool.Exec(context.Background(), `UPDATE simulation_jobs SET status = 'running', attempt = 2, heartbeat_at = now() - interval '1 hour' WHERE id = $1`, job.ID); err != nil {
			t.Fatal(err)
		}
		p.Recover(context.Background())
		failed := waitStatus(t, q, job.ID, StatusFailed)
		if failed.ErrorText == nil || runStatus(t, pool, job) != StatusFailed {
			t.Fatalf("exhausted job %+v", failed)
		}
	})
}
