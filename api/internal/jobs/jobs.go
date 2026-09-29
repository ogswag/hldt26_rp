// Package jobs runs simulation jobs from the Postgres queue inside the API process.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"moscow_hackathon_2026/api/internal/db"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"

	staleText = "Обработчик остановился во время расчёта. Повторите запуск."
)

// ErrCanceled is returned by a heartbeat when the user canceled the job.
var ErrCanceled = errors.New("jobs: canceled")

// PermanentError marks a failure that a retry cannot fix. Msg is shown to the user.
type PermanentError struct {
	Msg string
	Err error
}

func (e *PermanentError) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent wraps err with a user-facing message.
func Permanent(msg string, err error) error {
	return &PermanentError{Msg: msg, Err: err}
}

// Progress is updated by the handler; the pool flushes it on heartbeat.
type Progress struct {
	pct  atomic.Int32
	done atomic.Int32
}

func (p *Progress) Set(pct, done int) {
	if pct < 0 {
		pct = 0
	}
	if pct > 99 {
		pct = 99
	}
	p.pct.Store(int32(pct))
	p.done.Store(int32(done))
}

// Handler executes one claimed job. On success it must finish the job with Complete inside its own transaction.
type Handler interface {
	Run(ctx context.Context, job db.SimulationJob, progress *Progress) error
}

type Config struct {
	Workers       int
	Poll          time.Duration
	Heartbeat     time.Duration
	StaleAfter    time.Duration
	JobTimeout    time.Duration
	JanitorEvery  time.Duration
	MaintainEvery time.Duration
}

func (c Config) withDefaults() Config {
	if c.Workers <= 0 {
		c.Workers = 2
	}
	if c.Poll <= 0 {
		c.Poll = time.Second
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = time.Second
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = 30 * time.Second
	}
	if c.JobTimeout <= 0 {
		c.JobTimeout = 10 * time.Minute
	}
	if c.JanitorEvery <= 0 {
		c.JanitorEvery = 15 * time.Second
	}
	if c.MaintainEvery <= 0 {
		c.MaintainEvery = 10 * time.Minute
	}
	return c
}

type Pool struct {
	q        *db.Queries
	h        Handler
	log      *slog.Logger
	cfg      Config
	wake     chan struct{}
	maintain []func(context.Context) error
}

func New(q *db.Queries, h Handler, log *slog.Logger, cfg Config) *Pool {
	return &Pool{q: q, h: h, log: log, cfg: cfg.withDefaults(), wake: make(chan struct{}, 1)}
}

// OnMaintain registers periodic housekeeping such as artifact retention.
func (p *Pool) OnMaintain(fn func(context.Context) error) {
	p.maintain = append(p.maintain, fn)
}

// Notify wakes an idle worker after a job was enqueued.
func (p *Pool) Notify() {
	if p == nil {
		return
	}
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Start launches workers and the janitor. The returned function stops them and waits.
func (p *Pool) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	for i := 0; i < p.cfg.Workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			p.worker(ctx, n)
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.janitor(ctx)
	}()
	return func() {
		cancel()
		wg.Wait()
	}
}

func (p *Pool) worker(ctx context.Context, n int) {
	ticker := time.NewTicker(p.cfg.Poll)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := p.q.ClaimSimulationJob(ctx)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
				p.log.Error("jobs.claim", "op", "jobs.claim", "worker", n, "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-p.wake:
			case <-ticker.C:
			}
			continue
		}
		p.sync(ctx, job.ID)
		p.execute(ctx, job)
	}
}

func (p *Pool) execute(parent context.Context, job db.SimulationJob) {
	log := p.log.With("op", "jobs.run", "job_id", job.ID.String(), "project_id", job.ProjectID.String(), "attempt", job.Attempt)
	runCtx, cancelRun := context.WithTimeout(context.WithoutCancel(parent), p.cfg.JobTimeout)
	defer cancelRun()
	var canceled atomic.Bool
	progress := &Progress{}
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		t := time.NewTicker(p.cfg.Heartbeat)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-parent.Done():
				cancelRun()
				return
			case <-t.C:
				at, err := p.q.HeartbeatSimulationJob(runCtx, db.HeartbeatSimulationJobParams{
					ID:               job.ID,
					ProgressPct:      progress.pct.Load(),
					ReplicationsDone: progress.done.Load(),
				})
				if errors.Is(err, pgx.ErrNoRows) || (err == nil && at.Valid) {
					canceled.Store(true)
					cancelRun()
					return
				}
				if err != nil && runCtx.Err() == nil {
					log.Warn("jobs.heartbeat", "err", err)
				}
			}
		}
	}()
	start := time.Now()
	err := p.h.Run(runCtx, job, progress)
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	cancelRun()
	<-hbDone
	ctx, cancelFinish := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFinish()
	switch {
	case err == nil:
		log.Info("jobs.done", "ms", time.Since(start).Milliseconds())
	case canceled.Load() || errors.Is(err, ErrCanceled):
		p.finish(ctx, job, StatusCanceled, progress, nil)
		log.Info("jobs.canceled", "ms", time.Since(start).Milliseconds())
	case parent.Err() != nil && !timedOut:
		if rqErr := p.q.RequeueSimulationJob(ctx, db.RequeueSimulationJobParams{ID: job.ID, Refund: true}); rqErr != nil {
			log.Error("jobs.requeue", "err", rqErr)
		}
		p.sync(ctx, job.ID)
		log.Info("jobs.requeued_on_shutdown")
	case timedOut:
		msg := fmt.Sprintf("Симуляция не уложилась в %d мин, сократите горизонт, число повторов или флот.", int(p.cfg.JobTimeout.Minutes()))
		p.finish(ctx, job, StatusFailed, progress, &msg)
		log.Warn("jobs.timeout", "ms", time.Since(start).Milliseconds())
	default:
		var perm *PermanentError
		if errors.As(err, &perm) {
			msg := perm.Msg
			p.finish(ctx, job, StatusFailed, progress, &msg)
			log.Warn("jobs.failed", "err", err)
			return
		}
		if job.Attempt < job.MaxAttempts {
			msg := "Временная ошибка, задание будет повторено."
			if rqErr := p.q.RequeueSimulationJob(ctx, db.RequeueSimulationJobParams{ID: job.ID, ErrorText: &msg}); rqErr != nil {
				log.Error("jobs.requeue", "err", rqErr)
			}
			p.sync(ctx, job.ID)
			log.Warn("jobs.retry", "err", err)
			p.Notify()
			return
		}
		msg := fmt.Sprintf("Не удалось выполнить симуляцию за %d попытки. Повторите запуск позже.", job.MaxAttempts)
		p.finish(ctx, job, StatusFailed, progress, &msg)
		log.Error("jobs.failed", "err", err)
	}
}

func (p *Pool) finish(ctx context.Context, job db.SimulationJob, status string, progress *Progress, msg *string) {
	if _, err := p.q.FinishSimulationJob(ctx, db.FinishSimulationJobParams{
		ID:               job.ID,
		Status:           status,
		ProgressPct:      progress.pct.Load(),
		ReplicationsDone: progress.done.Load(),
		ErrorText:        msg,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		p.log.Error("jobs.finish", "op", "jobs.finish", "job_id", job.ID.String(), "err", err)
	}
	p.sync(ctx, job.ID)
}

func (p *Pool) sync(ctx context.Context, id uuid.UUID) {
	if err := p.q.SyncSimulationRunStatus(ctx, id); err != nil {
		p.log.Error("jobs.sync", "op", "jobs.sync", "job_id", id.String(), "err", err)
	}
}

func (p *Pool) janitor(ctx context.Context) {
	t := time.NewTicker(p.cfg.JanitorEvery)
	defer t.Stop()
	lastMaintain := time.Time{}
	for {
		p.Recover(ctx)
		if time.Since(lastMaintain) >= p.cfg.MaintainEvery {
			lastMaintain = time.Now()
			for _, fn := range p.maintain {
				if err := fn(ctx); err != nil && ctx.Err() == nil {
					p.log.Error("jobs.maintain", "op", "jobs.maintain", "err", err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Recover requeues running jobs whose worker stopped sending heartbeats.
func (p *Pool) Recover(ctx context.Context) {
	ids, err := p.q.RecoverStaleSimulationJobs(ctx, db.RecoverStaleSimulationJobsParams{
		FailedText:  staleText,
		StaleBefore: pgtype.Timestamptz{Time: time.Now().Add(-p.cfg.StaleAfter), Valid: true},
	})
	if err != nil {
		if ctx.Err() == nil {
			p.log.Error("jobs.recover", "op", "jobs.recover", "err", err)
		}
		return
	}
	for _, id := range ids {
		p.sync(ctx, id)
		p.log.Warn("jobs.recovered", "op", "jobs.recover", "job_id", id.String())
	}
	if len(ids) > 0 {
		p.Notify()
	}
}

// Complete marks a running job succeeded. Call it inside the transaction that stores results.
func Complete(ctx context.Context, q *db.Queries, jobID uuid.UUID) error {
	if _, err := q.FinishSimulationJob(ctx, db.FinishSimulationJobParams{ID: jobID, Status: StatusSucceeded}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCanceled
		}
		return fmt.Errorf("jobs.complete: %w", err)
	}
	if err := q.SyncSimulationRunStatus(ctx, jobID); err != nil {
		return fmt.Errorf("jobs.complete.sync: %w", err)
	}
	return nil
}

// Active reports whether a status still needs a worker.
func Active(status string) bool {
	return status == StatusQueued || status == StatusRunning
}
