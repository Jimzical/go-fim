// Package agent owns the two top-level entry points the CLI dispatches to:
// Run (cron-driven scan + report) and Setup (one-shot registration handshake).
// cmd/go-fim/main.go is a thin shell — flag parsing lives there, business
// logic lives here.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"

	"github.com/Jimzical/go-fim/internal/client"
	"github.com/Jimzical/go-fim/internal/config"
	"github.com/Jimzical/go-fim/internal/hasher"
	"github.com/Jimzical/go-fim/internal/logger"
	"github.com/Jimzical/go-fim/internal/report"
	"github.com/Jimzical/go-fim/internal/store"
	"github.com/Jimzical/go-fim/internal/walker"
)

// Run is the default scan loop: load config, walk + hash + diff, write a
// report locally, then POST it (or queue for replay if the server is down).
func Run(cfgPath string, verboseFlag bool, local bool) (err error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if local && errors.Is(err, os.ErrNotExist) {
			cfg, err = config.Default()
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if local {
		cfg.ServerURL = ""
	}
	if cfg.ServerURL != "" && cfg.AgentName == "" {
		return fmt.Errorf("config %q: 'agent_name' is required when 'server_url' is set (use -local to skip)", cfgPath)
	}

	log := logger.New(verboseFlag)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer deferClose(db, &err)()
	log.Info("opened db", "path", cfg.DBPath)

	agentID := cfg.AgentID
	if agentID == "" {
		agentID, err = store.AgentID(db)
		if err != nil {
			return err
		}
	}
	log.Info("agent id", "id", agentID)

	apiToken, err := store.GetAPIToken(db)
	if err != nil {
		return err
	}

	var httpClient *client.Client
	if cfg.ServerURL != "" {
		httpClient = client.New(cfg.ServerURL, apiToken, cfg.InsecureSkipVerify)
	}

	// Drain any reports queued by previous runs before kicking off the fresh
	// scan. If the server is still down, replayPending halts on the first
	// ErrUnreachable; we'll skip the fresh POST attempt below to avoid a
	// per-cron-tick timeout cascade.
	serverDown := false
	if httpClient != nil {
		serverDown = replayPending(log, httpClient, cfg.HistoryDir)
	}

	total, summary, err := runScan(log, cfg, db)
	if err != nil {
		return err
	}
	printSummary(total, summary)

	rep := report.FromSummary(total, summary)
	rep.AgentID = agentID
	rep.AgentName = cfg.AgentName
	rep.ScanPath = cfg.Path
	w := report.Writer{Dir: cfg.HistoryDir, MaxN: report.HistoryMaxN}
	path, err := w.Save(rep)
	if err != nil {
		return fmt.Errorf("save report: %w", err)
	}
	log.Info("wrote report", "path", path)

	if httpClient != nil {
		sendOrQueue(log, httpClient, rep, path, serverDown)
	}
	return nil
}

// runScan drives the three-stage pipeline: walker → hasher → store. Each stage
// runs in its own goroutine; channels propagate results, errgroup propagates
// cancellation. Returns the total file count and the diff summary.
func runScan(log *slog.Logger, cfg *config.Config, db *bbolt.DB) (int64, store.Summary, error) {
	g, ctx := errgroup.WithContext(context.Background())
	metaCh := make(chan walker.FileMeta, 1024)
	hashedCh := make(chan walker.FileMeta, 1024)

	var totalFiles int64
	g.Go(func() error {
		defer close(metaCh) // signals "no more files" to the hasher
		n, err := walker.Walk(ctx, log, cfg.Path, cfg.Exclude, []string{cfg.DBPath, cfg.HistoryDir}, metaCh)
		atomic.StoreInt64(&totalFiles, n)
		return err
	})

	g.Go(func() error {
		defer close(hashedCh) // signals "no more entries" to the store writer
		return hasher.Run(ctx, log, metaCh, hashedCh)
	})

	var summary store.Summary
	g.Go(func() error {
		s, err := store.Run(ctx, log, db, hashedCh)
		summary = s
		return err
	})

	if err := g.Wait(); err != nil {
		return 0, store.Summary{}, fmt.Errorf("scan: %w", err)
	}
	return atomic.LoadInt64(&totalFiles), summary, nil
}

// sendOrQueue posts the fresh report or files it for replay. If serverDown is
// true the replay loop already proved the server is unreachable, so we skip
// the timeout and queue directly. Errors are logged, not returned: a failed
// POST shouldn't fail the cron-driven run.
func sendOrQueue(log *slog.Logger, c *client.Client, rep report.Report, path string, serverDown bool) {
	if serverDown {
		queueForReplay(log, path, "server down")
		return
	}
	err := c.SendReport(rep)
	switch {
	case err == nil:
		log.Info("report sent")
	case errors.Is(err, client.ErrUnreachable):
		queueForReplay(log, path, "unreachable")
	default: // *HTTPError or other non-retryable
		log.Error("dropping report on protocol error", "err", err)
	}
}

func queueForReplay(log *slog.Logger, path, reason string) {
	if err := report.MarkUnsent(path); err != nil {
		log.Error("mark unsent", "err", err)
		return
	}
	log.Warn("queued for replay", "path", path, "reason", reason)
}

// replayPending drains queued reports left by previous runs that failed to
// POST. Replays oldest-first so the server's timeline reflects scan order.
// Returns true if a replay attempt hit ErrUnreachable — the caller uses this
// to short-circuit the fresh POST and avoid a second 5s timeout per cron tick.
// Malformed pending files and 4xx responses are dropped (logged + removed).
func replayPending(log *slog.Logger, c *client.Client, dir string) bool {
	pending, err := report.ListUnsent(dir)
	if err != nil {
		log.Warn("list pending reports", "err", err)
		return false
	}
	if len(pending) == 0 {
		return false
	}
	log.Info("replaying pending reports", "count", len(pending))

	for i, p := range pending {
		rep, err := report.LoadFromFile(p)
		if err != nil {
			log.Warn("dropping malformed pending report", "path", p, "err", err)
			_ = os.Remove(p)
			continue
		}
		err = c.SendReport(rep)
		switch {
		case err == nil:
			if rmErr := os.Remove(p); rmErr != nil {
				log.Warn("replayed but couldn't remove queue file", "path", p, "err", rmErr)
			} else {
				log.Info("replayed", "path", p)
			}
		case errors.Is(err, client.ErrUnreachable):
			log.Warn("server still unreachable; halting replay", "remaining", len(pending)-i)
			return true
		default: // *HTTPError or unexpected
			log.Error("dropping pending report on protocol error", "path", p, "err", err)
			_ = os.Remove(p)
		}
	}
	return false
}

// printSummary writes the human-readable diff to stdout.
func printSummary(total int64, s store.Summary) {
	fmt.Printf("scanned: %d files\n", total)
	if len(s.Changes) == 0 {
		fmt.Println("no changes")
		return
	}
	fmt.Printf("changes: %d (%d created, %d modified, %d deleted)\n\n",
		len(s.Changes), s.NumCreated, s.NumModified, s.NumDeleted)
	for _, c := range s.Changes {
		fmt.Printf("  %s %s\n", c.Kind.Symbol(), c.Path)
	}
}
