// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunRejectsShortDuration(t *testing.T) {
	// duration < minDuration must be rejected before run() does anything
	// OS-specific (mmap, /proc/meminfo, DAMON) — this is the one path
	// through run() that's safe to exercise without root or a real
	// kernel.
	err := run(context.Background(), "8GiB", "2GiB", time.Minute, false)
	if err == nil {
		t.Fatal("run with --duration 1m: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--duration must be at least") {
		t.Errorf("run with --duration 1m: error = %q, want it to mention the --duration floor", err)
	}
}
