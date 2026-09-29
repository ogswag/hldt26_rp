package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"moscow_hackathon_2026/api/internal/auth"
	"moscow_hackathon_2026/api/internal/config"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/httpserver"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/internal/jobs"
	"moscow_hackathon_2026/api/internal/mail"
	"moscow_hackathon_2026/api/internal/realtime"
	"moscow_hackathon_2026/api/internal/simjobs"
	"moscow_hackathon_2026/api/migrations"
)

const (
	mailEvery     = 2 * time.Second
	mailRetention = 30 * 24 * time.Hour
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("exit", "op", "main", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db.parse: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("db.pool: %w", err)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("db.ping: %w", err)
	}

	if err := migrations.Up(cfg.DatabaseURL); err != nil {
		return err
	}

	q := db.New(pool)
	seedCtx, seedCancel := context.WithTimeout(ctx, 60*time.Second)
	defer seedCancel()
	seed, err := importers.Seed(seedCtx, pool, q, cfg.CatalogCSV, cfg.CatalogSpecs)
	if err != nil {
		return err
	}
	log.Info("catalog.seed", "op", "catalog.seed",
		"inserted", seed.CatalogInserted,
		"split_inserted", seed.SplitInserted,
		"extra", seed.ExtraInserted,
		"edited_skipped", seed.EditedSkipped,
		"csv_rows", seed.Inventory.DataRows,
		"unique_ids", seed.Inventory.UniqueIDs,
		"duplicated_ids", seed.Inventory.DuplicatedIDs,
		"extra_rows", seed.Inventory.ExtraRows,
		"price_conflicts", seed.Inventory.PriceConflicts,
	)

	interrupted, err := q.FinishInterruptedCatalogImports(seedCtx)
	if err != nil {
		return fmt.Errorf("catalog.import.interrupted: %w", err)
	}
	if interrupted > 0 {
		log.Info("catalog.import.interrupted", "op", "catalog.import", "finished", interrupted)
	}

	if err := auth.SeedDemos(seedCtx, q, cfg.DemoUserEmail, cfg.DemoUserPassword, cfg.DemoAdminEmail, cfg.DemoAdminPassword, cfg.BcryptCost); err != nil {
		return err
	}

	var sender mail.Sender = &mail.Capture{}
	if cfg.MailMode == config.MailSMTP {
		sender = mail.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, From: cfg.MailFrom, User: cfg.SMTPUser, Password: cfg.SMTPPassword}
	}
	outbox := mail.NewOutbox(q, sender, log)

	sims := simjobs.New(q, log, cfg.SimLogRetention)
	workers := jobs.New(q, sims, log, jobs.Config{Workers: cfg.SimWorkers, JobTimeout: cfg.SimJobTimeout})
	workers.OnMaintain(sims.Maintain)
	workers.OnMaintain(func(ctx context.Context) error {
		n, err := q.DeleteStaleSessions(ctx, pgtype.Timestamptz{Time: time.Now(), Valid: true})
		if err != nil {
			return fmt.Errorf("auth.sessions.purge: %w", err)
		}
		if n > 0 {
			log.Info("auth.sessions.purge", "op", "auth.sessions.purge", "deleted", n)
		}
		return nil
	})
	workers.OnMaintain(func(ctx context.Context) error {
		n, err := q.DeleteOldProjectOperations(ctx, pgtype.Timestamptz{Time: time.Now().Add(-httpserver.OpsRetention), Valid: true})
		if err != nil {
			return fmt.Errorf("ops.purge: %w", err)
		}
		if n > 0 {
			log.Info("ops.purge", "op", "ops.purge", "deleted", n)
		}
		return nil
	})
	workers.OnMaintain(func(ctx context.Context) error {
		n, err := httpserver.PurgeTrash(ctx, q, time.Now().Add(-cfg.TrashRetention), 100)
		if err != nil {
			return err
		}
		if n > 0 {
			log.Info("projects.purge", "op", "projects.purge", "deleted", n)
		}
		return nil
	})
	workers.OnMaintain(func(ctx context.Context) error {
		if _, err := q.DeleteOldAuditEvents(ctx, pgtype.Timestamptz{Time: time.Now().Add(-cfg.AuditRetention), Valid: true}); err != nil {
			return fmt.Errorf("audit.purge: %w", err)
		}
		if _, err := q.DeleteStaleEmailTokens(ctx, pgtype.Timestamptz{Time: time.Now(), Valid: true}); err != nil {
			return fmt.Errorf("mail.tokens.purge: %w", err)
		}
		if _, err := q.DeleteOldEmails(ctx, pgtype.Timestamptz{Time: time.Now().Add(-mailRetention), Valid: true}); err != nil {
			return fmt.Errorf("mail.outbox.purge: %w", err)
		}
		return nil
	})
	stopWorkers := workers.Start(ctx)
	defer stopWorkers()

	mailCtx, stopMail := context.WithCancel(ctx)
	defer stopMail()
	go sendMail(mailCtx, outbox, log)

	hub := realtime.New(cfg.DatabaseURL, log)
	hubCtx, stopHub := context.WithCancel(ctx)
	defer stopHub()
	go hub.Run(hubCtx)

	srv := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           httpserver.New(cfg, log, q, sims, workers, hub),
		ReadHeaderTimeout: 5 * time.Second,
	}
	// Shutdown waits for idle connections; live streams never idle, so they are ended first.
	srv.RegisterOnShutdown(hub.CloseAll)

	errCh := make(chan error, 1)
	go func() {
		log.Info("listen", "op", "http.listen", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info("shutdown", "op", "http.shutdown", "signal", sig.String())
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http.listen: %w", err)
		}
		return nil
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	err = srv.Shutdown(shutCtx)
	stopWorkers()
	return err
}

// sendMail delivers queued letters. It runs beside the job pool, on a shorter tick than housekeeping.
func sendMail(ctx context.Context, outbox *mail.Outbox, log *slog.Logger) {
	t := time.NewTicker(mailEvery)
	defer t.Stop()
	for {
		if err := outbox.Send(ctx); err != nil && ctx.Err() == nil {
			log.Error("mail.outbox", "op", "mail.send", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
