// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

//go:build damon_live

// This file only runs against a real kernel with CONFIG_DAMON_SYSFS
// enabled (>=5.18, tried_regions readout needs >=6.2) — it writes to
// /sys/kernel/mm/damon/admin for real. It will not run in CI (GitHub
// Actions' ubuntu-latest and a stock Hetzner Ubuntu 24.04 image were both
// checked and neither has CONFIG_DAMON_SYSFS built in). Run explicitly,
// as root, on a DAMON-capable box:
//
//	go test -tags damon_live ./pkg/damon/...
package damon

import (
	"testing"
	"time"
)

func TestLiveStartThenStop(t *testing.T) {
	caps, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !caps.Sysfs {
		t.Skip("no DAMON sysfs interface on this kernel")
	}

	regions, err := ParseIomem()
	if err != nil {
		t.Fatalf("ParseIomem: %v (must run as root)", err)
	}
	if len(regions) == 0 {
		t.Fatal("ParseIomem: got no System RAM ranges")
	}

	sess, err := Start(Config{
		Ops:        "paddr",
		SampleUS:   5_000,
		AggrUS:     100_000,
		UpdateUS:   1_000_000,
		MinRegions: 10,
		MaxRegions: 1_000,
		Regions:    regions,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	state, err := readSysfsFile("/sys/kernel/mm/damon/admin/kdamonds/0/state")
	if err != nil {
		t.Fatalf("read state after Start: %v", err)
	}
	if state != "on" {
		t.Fatalf("state after Start = %q, want \"on\"", state)
	}

	if err := sess.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	state, err = readSysfsFile("/sys/kernel/mm/damon/admin/kdamonds/0/state")
	if err != nil {
		t.Fatalf("read state after Stop: %v", err)
	}
	if state != "off" {
		t.Fatalf("state after Stop = %q, want \"off\"", state)
	}
}

// TestLiveSnapshotMonitoredBytesApproxSystemRAM is this package's core
// acceptance bar: if DAMON can't see the full monitored range, nothing
// built on top of it (the cold-page histogram, downstream correctness
// checks) can be trusted either.
func TestLiveSnapshotMonitoredBytesApproxSystemRAM(t *testing.T) {
	caps, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !caps.TriedRegions {
		t.Skip("this kernel doesn't support the tried_regions readout (needs >=6.2)")
	}

	regions, err := ParseIomem()
	if err != nil {
		t.Fatalf("ParseIomem: %v (must run as root)", err)
	}
	var wantBytes uint64
	for _, r := range regions {
		wantBytes += r.End - r.Start + 1 // AddrRange.End is inclusive
	}

	sess, err := Start(Config{
		Ops:        "paddr",
		SampleUS:   5_000,
		AggrUS:     100_000,
		UpdateUS:   1_000_000,
		MinRegions: 10,
		MaxRegions: 1_000,
		Regions:    regions,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := sess.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	// Give DAMON at least one full aggregation interval (100ms) before
	// asking for a tried_regions snapshot.
	time.Sleep(200 * time.Millisecond)

	tried, err := sess.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(tried) == 0 {
		t.Fatal("Snapshot: got no regions")
	}

	var gotBytes uint64
	for _, r := range tried {
		gotBytes += r.End - r.Start
	}

	t.Logf("configured %d bytes across %d System RAM ranges, DAMON reports %d bytes across %d tried_regions", wantBytes, len(regions), gotBytes, len(tried))

	// Confirmed live (2026-08-18, 4GiB box): DAMON internally rounds/aligns
	// tracked region boundaries, so this isn't exact — off by 1024 bytes
	// out of ~4GiB in that run, an inherent consequence of region
	// granularity rather than a bug. 0.01% is generous headroom above the
	// observed discrepancy while still catching a real "DAMON isn't
	// seeing most of RAM" failure.
	var diff uint64
	if gotBytes > wantBytes {
		diff = gotBytes - wantBytes
	} else {
		diff = wantBytes - gotBytes
	}
	if maxDiff := wantBytes / 10_000; diff > maxDiff {
		t.Errorf("monitored bytes = %d, want ~%d (configured System RAM total), diff %d exceeds tolerance %d", gotBytes, wantBytes, diff, maxDiff)
	}
}
