// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/memaudit/memaudit/internal/config"
	"github.com/memaudit/memaudit/pkg/model"
	"github.com/memaudit/memaudit/internal/ship"
	"github.com/memaudit/memaudit/internal/spool"
)

// TestRunBundleModeCollectsAndFlushes proves the full wiring: goroutines
// actually tick, write to the real spool package, and a context cancel
// flushes the partial segment to disk — all without a shipper (bundle
// mode), matching ship.Shipper's own contract of "never invoked in
// bundle mode."
func TestRunBundleModeCollectsAndFlushes(t *testing.T) {
	dir := t.TempDir()
	sp, err := spool.Open(dir, spool.Options{Site: "acme", Host: "host-a"})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	err = Run(ctx, Config{
		Site:       "acme",
		Host:       "host-a",
		ProcRoot:   "../../testdata/container-linux-6.12/proc",
		SysRoot:    "../../testdata/container-linux-6.12/sys",
		Interval:   20 * time.Millisecond,
		Collectors: config.CollectorsConfig{}, // Cgroup disabled by zero value — not needed for this test
		Spool:      sp,
		Jitter:     func(time.Duration) time.Duration { return 0 }, // deterministic: every source ticks immediately
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	segs, err := sp.Segments()
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) == 0 {
		t.Fatal("Run returned with no spooled segments — shutdown flush didn't happen")
	}

	gotTypes := map[string]bool{}
	for _, seg := range segs {
		for _, e := range readEnvelopes(t, seg) {
			gotTypes[e.Type] = true
		}
	}
	for _, want := range []string{"host_mem", "vmstat", "psi", "numa_mem"} {
		if !gotTypes[want] {
			t.Errorf("no %s envelope spooled; got types %v", want, gotTypes)
		}
	}
}

// TestRunConcurrentWritesNoRace runs the fast-tier sources (host_mem,
// vmstat, psi all tick every Interval) on a very short interval for long
// enough to force many overlapping ticks across goroutines, all writing
// through the same *spool.Spool. It exists to catch a concurrent-write
// data race in Spool.Write under `go test -race`, since Spool itself has
// no internal locking — Run must serialize writes on its own.
func TestRunConcurrentWritesNoRace(t *testing.T) {
	dir := t.TempDir()
	sp, err := spool.Open(dir, spool.Options{Site: "acme", Host: "host-b"})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = Run(ctx, Config{
		Site:       "acme",
		Host:       "host-b",
		ProcRoot:   "../../testdata/container-linux-6.12/proc",
		SysRoot:    "../../testdata/container-linux-6.12/sys",
		Interval:   2 * time.Millisecond,
		Collectors: config.CollectorsConfig{},
		Spool:      sp,
		Jitter:     func(time.Duration) time.Duration { return 0 },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	segs, err := sp.Segments()
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) == 0 {
		t.Fatal("expected at least one spooled segment")
	}
}

// TestRunRejectsNonPositiveInterval proves Run validates Interval before
// starting any goroutine: time.NewTicker panics on a non-positive
// duration, and that panic would happen inside a source goroutine where
// the caller of Run could never catch it.
func TestRunRejectsNonPositiveInterval(t *testing.T) {
	dir := t.TempDir()
	sp, err := spool.Open(dir, spool.Options{Site: "acme", Host: "host-c"})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}
	defer func() { _ = sp.Close() }()

	tests := []struct {
		name     string
		interval time.Duration
	}{
		{"zero", 0},
		{"negative", -1 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Run(context.Background(), Config{
				Site:     "acme",
				Host:     "host-c",
				Interval: tt.interval,
				Spool:    sp,
			})
			if err == nil {
				t.Fatal("expected an error for a non-positive Interval, got nil")
			}
		})
	}
}

// TestRunRejectsNilSpool proves Run validates Spool before starting any
// goroutine: a nil Spool would otherwise nil-deref inside a source
// goroutine on the first tick, uncatchably crashing the process.
func TestRunRejectsNilSpool(t *testing.T) {
	err := Run(context.Background(), Config{
		Site:     "acme",
		Host:     "host-d",
		Interval: time.Second,
		Spool:    nil,
	})
	if err == nil {
		t.Fatal("expected an error for a nil Spool, got nil")
	}
}

// TestRunShipsSegmentsWhenShipperConfigured exercises the previously
// untested Shipper != nil path end to end against a real ship.Shipper
// and a real HTTP server: sources tick, write to the spool, the spool
// rotates segments (RotateBytes: 1 forces rotation on every write so a
// shippable segment exists well before shutdown), the ship-drain
// goroutine ships them, and the final shutdown drain mops up whatever's
// left. Asserts the server actually received requests and that no
// segments are left behind afterward.
func TestRunShipsSegmentsWhenShipperConfigured(t *testing.T) {
	dir := t.TempDir()
	sp, err := spool.Open(dir, spool.Options{Site: "acme", Host: "host-e", RotateBytes: 1})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}

	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	shipper := ship.New(ship.Config{URL: srv.URL, Token: "test-token"})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err = Run(ctx, Config{
		Site:       "acme",
		Host:       "host-e",
		ProcRoot:   "../../testdata/container-linux-6.12/proc",
		SysRoot:    "../../testdata/container-linux-6.12/sys",
		Interval:   10 * time.Millisecond,
		Collectors: config.CollectorsConfig{},
		Spool:      sp,
		Shipper:    shipper,
		Jitter:     func(time.Duration) time.Duration { return 0 },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	got := requests
	mu.Unlock()
	if got == 0 {
		t.Fatal("expected the shipper to have made at least one request, got 0")
	}

	segs, err := sp.Segments()
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) != 0 {
		t.Errorf("expected every segment to be shipped and removed, got %d left: %v", len(segs), segs)
	}
}

// TestRunShutdownDrainRespectsTimeout proves the fix for the hang this
// review caught: against an endpoint that always returns a retryable
// failure, the pre-fix code called Shipper.Run with context.Background()
// for the final drain, which retries with backoff forever and would
// never return. With ShutdownTimeout bounding that final attempt, Run
// must return promptly (well under a second, not 15s or forever) once
// the shutdown drain's own deadline is hit.
func TestRunShutdownDrainRespectsTimeout(t *testing.T) {
	dir := t.TempDir()
	sp, err := spool.Open(dir, spool.Options{Site: "acme", Host: "host-f", RotateBytes: 1})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // always retryable, never succeeds
	}))
	defer srv.Close()

	shipper := ship.New(ship.Config{
		URL:        srv.URL,
		Token:      "test-token",
		MinBackoff: 20 * time.Millisecond,
		MaxBackoff: 20 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = Run(ctx, Config{
		Site:            "acme",
		Host:            "host-f",
		ProcRoot:        "../../testdata/container-linux-6.12/proc",
		SysRoot:         "../../testdata/container-linux-6.12/sys",
		Interval:        10 * time.Millisecond,
		Collectors:      config.CollectorsConfig{},
		Spool:           sp,
		Shipper:         shipper,
		Jitter:          func(time.Duration) time.Duration { return 0 },
		ShutdownTimeout: 100 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from the bounded shutdown drain against an always-503 server, got nil")
	}
	// The spool is already closed and on disk at this point, so the
	// caller must be able to tell this apart from a real run failure and
	// stop cleanly instead of exiting non-zero.
	if !errors.Is(err, ErrShutdownDrainIncomplete) {
		t.Errorf("Run error = %v, want it to wrap ErrShutdownDrainIncomplete", err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("Run took %v to return after context cancellation; shutdown drain did not respect ShutdownTimeout", elapsed)
	}
}

// readEnvelopes decompresses seg (a .jsonl.zst spool segment) and
// decodes each line. This duplicates internal/spool/spool_test.go's own
// decompressSegment helper (same zstd-reader-plus-line-scan shape) —
// small enough that duplication beats a cross-package test dependency.
func readEnvelopes(t *testing.T, seg string) []model.Envelope {
	t.Helper()
	f, err := os.Open(seg)
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}
	defer func() { _ = f.Close() }()

	dec, err := zstd.NewReader(f)
	if err != nil {
		t.Fatalf("zstd.NewReader: %v", err)
	}
	defer dec.Close()

	raw, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("decompress %s: %v", seg, err)
	}

	var out []model.Envelope
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		var e model.Envelope
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal envelope from %s: %v", seg, err)
		}
		out = append(out, e)
	}
	return out
}
