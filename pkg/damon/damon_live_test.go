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

import "testing"

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
