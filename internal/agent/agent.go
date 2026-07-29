// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package agent runs the tick loop: one goroutine per collector on a
// jittered ticker writing to the spool, plus one ship-drain goroutine
// when shipping is configured. A context cancel stops every goroutine,
// flushes the spool, and (if shipping) makes one last attempt to drain
// it before Run returns.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/memaudit/memaudit/internal/config"
	"github.com/memaudit/memaudit/internal/k8s"
	"github.com/memaudit/memaudit/internal/ship"
	"github.com/memaudit/memaudit/internal/spool"
)

// defaultShutdownTimeout bounds the final ship-drain attempt Run makes
// after every source goroutine has stopped. Without a bound, a
// persistently unreachable or failing ingest endpoint would retry with
// backoff forever against context.Background(), hanging shutdown
// indefinitely even though the spool itself is already safely flushed.
const defaultShutdownTimeout = 15 * time.Second

// ErrShutdownDrainIncomplete reports that only the best-effort final
// ship drain failed or ran out of time — the common case being a host
// shutdown that tears down networking while the unit is stopping. Every
// goroutine has stopped and the spool is already closed and durably on
// disk by then, so nothing is lost: the leftover segments ship on the
// next start. Callers should treat it as a clean stop, not a failure.
var ErrShutdownDrainIncomplete = errors.New("shutdown ship drain did not complete")

// Config configures a Run.
type Config struct {
	Site, Host        string
	ProcRoot, SysRoot string
	// Interval is the base tick (the config file's interval_s, already
	// converted to a Duration). Medium/slow sources tick at 2x/4x this.
	Interval   time.Duration
	Collectors config.CollectorsConfig
	// K8sClient enables cgroup_mem pod enrichment; nil disables it.
	K8sClient k8s.Client
	Spool     *spool.Spool
	// Shipper is nil in bundle mode: Run then never starts a ship
	// goroutine, matching ship.Shipper's own "never invoked" contract.
	Shipper *ship.Shipper
	// Now and Jitter default to time.Now and a real random jitter; tests
	// override both for determinism.
	Now    func() time.Time
	Jitter func(time.Duration) time.Duration
	// ShutdownTimeout bounds the final ship-drain attempt Run makes, once
	// every goroutine has stopped, before returning. Defaults to 15s;
	// tests override it to keep a failure-path test fast instead of
	// waiting out the real default.
	ShutdownTimeout time.Duration
}

// Run starts one goroutine per source plus, if Shipper is set, a
// ship-drain goroutine, and blocks until ctx is cancelled. On
// cancellation it stops every goroutine, flushes the spool (rotating any
// partial segment to a complete one), and if Shipper is set makes one
// final ship attempt — bounded by ShutdownTimeout, so a failing or
// unreachable ingest endpoint can't hang shutdown forever — before
// returning. A failure of that last drain alone is returned wrapped in
// ErrShutdownDrainIncomplete, so a caller can tell it apart from a real
// run failure.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Interval <= 0 {
		return fmt.Errorf("agent: Interval must be positive, got %v", cfg.Interval)
	}
	if cfg.Spool == nil {
		return fmt.Errorf("agent: Spool is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Jitter == nil {
		cfg.Jitter = realJitter
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}

	sources := buildSources(cfg.Collectors, cfg.ProcRoot, cfg.SysRoot, cfg.Site, cfg.Host, cfg.K8sClient, cfg.Now)

	// spool.Spool has no internal locking, but every source goroutine
	// below writes to the same *spool.Spool concurrently: serialize those
	// writes here so Run is the one responsible for making that safe.
	var writeMu sync.Mutex

	var wg sync.WaitGroup
	for _, src := range sources {
		wg.Add(1)
		go func(src source) {
			defer wg.Done()
			runSource(ctx, src, cfg.Interval*time.Duration(src.tier), cfg.Jitter, cfg.Spool, &writeMu)
		}(src)
	}

	if cfg.Shipper != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runShipper(ctx, cfg.Shipper, cfg.Spool, cfg.Interval, cfg.Jitter)
		}()
	}

	wg.Wait()

	if err := cfg.Spool.Close(); err != nil {
		return err
	}
	if cfg.Shipper != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := cfg.Shipper.Run(shutdownCtx, cfg.Spool.Segments); err != nil {
			return fmt.Errorf("%w: %w", ErrShutdownDrainIncomplete, err)
		}
	}
	return nil
}

func realJitter(d time.Duration) time.Duration {
	return time.Duration(rand.Int64N(int64(d) + 1)) //nolint:gosec // G404: jitter timing, not security-sensitive
}

// runSource collects src on a jittered ticker until ctx is cancelled. A
// collect or spool-write failure is logged and skipped — one bad tick
// never stops the loop. writeMu serializes writes to sp across every
// source goroutine, since spool.Spool is not safe for concurrent Write
// calls on its own.
func runSource(ctx context.Context, src source, interval time.Duration, jitter func(time.Duration) time.Duration, sp *spool.Spool, writeMu *sync.Mutex) {
	tick := func() {
		envs, err := src.collect()
		if err != nil {
			slog.Error("collect failed", "type", src.typ, "err", err)
			return
		}
		for _, e := range envs {
			writeMu.Lock()
			err := sp.Write(e)
			writeMu.Unlock()
			if err != nil {
				slog.Error("spool write failed", "type", src.typ, "err", err)
			}
		}
	}

	select {
	case <-time.After(jitter(interval)):
	case <-ctx.Done():
		return
	}
	tick()

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

// runShipper drains the spool on a jittered ticker until ctx is
// cancelled.
func runShipper(ctx context.Context, shipper *ship.Shipper, sp *spool.Spool, interval time.Duration, jitter func(time.Duration) time.Duration) {
	drain := func() {
		if err := shipper.Run(ctx, sp.Segments); err != nil && ctx.Err() == nil {
			slog.Error("ship failed", "err", err)
		}
	}

	select {
	case <-time.After(jitter(interval)):
	case <-ctx.Done():
		return
	}
	drain()

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			drain()
		}
	}
}
