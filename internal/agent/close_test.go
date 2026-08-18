// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/memaudit/memaudit/internal/model"
	"github.com/memaudit/memaudit/internal/spool"
)

func TestRunSourceAndCloseCallsCloseAfterShutdown(t *testing.T) {
	sp, err := spool.Open(t.TempDir(), spool.Options{Site: "acme", Host: "host-a"})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}

	var mu sync.Mutex
	var lastTick, closedAt time.Time
	closed := false

	src := source{
		typ:  "test",
		tier: tierFast,
		collect: func() ([]model.Envelope, error) {
			mu.Lock()
			lastTick = time.Now()
			mu.Unlock()
			return nil, nil
		},
		close: func() error {
			mu.Lock()
			closed = true
			closedAt = time.Now()
			mu.Unlock()
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	var writeMu sync.Mutex
	runSourceAndClose(ctx, src, 5*time.Millisecond, func(time.Duration) time.Duration { return 0 }, sp, &writeMu)

	mu.Lock()
	defer mu.Unlock()
	if !closed {
		t.Fatal("close was never called")
	}
	if closedAt.Before(lastTick) {
		t.Fatalf("close ran before the last tick completed: closedAt=%v lastTick=%v", closedAt, lastTick)
	}
}

func TestRunSourceAndCloseNilCloseIsNoop(t *testing.T) {
	sp, err := spool.Open(t.TempDir(), spool.Options{Site: "acme", Host: "host-a"})
	if err != nil {
		t.Fatalf("spool.Open: %v", err)
	}
	src := source{
		typ:     "test",
		tier:    tierFast,
		collect: func() ([]model.Envelope, error) { return nil, nil },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	var writeMu sync.Mutex
	runSourceAndClose(ctx, src, 5*time.Millisecond, func(time.Duration) time.Duration { return 0 }, sp, &writeMu) // must not panic
}
