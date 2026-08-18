// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import "testing"

func TestDetectAbsentIsAllFalseNotError(t *testing.T) {
	// No /sys/kernel/mm/damon/admin at all (CONFIG_DAMON_SYSFS off, or a
	// pre-5.18 kernel): expected, not an error, matching every other
	// collector's absence convention in this repo.
	got, err := DetectAt("../../testdata/edge-cases/damon-absent/proc", "../../testdata/edge-cases/vmstat-old-kernel/sys")
	if err != nil {
		t.Fatalf("DetectAt: %v", err)
	}
	want := Caps{}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDetectPre62IsStatsOnly(t *testing.T) {
	// Sysfs present, kernel 5.19 (the 5.18-6.1 rung): sysfs + paddr, no
	// tried_regions readout yet.
	got, err := DetectAt("../../testdata/edge-cases/damon-pre-6.2/proc", "../../testdata/edge-cases/damon-pre-6.2/sys")
	if err != nil {
		t.Fatalf("DetectAt: %v", err)
	}
	want := Caps{Sysfs: true, Paddr: true, TriedRegions: false}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDetectGoldenFullHistogram(t *testing.T) {
	// Real capture: Fedora box, kernel 7.1.6, CONFIG_DAMON_SYSFS on —
	// the >=6.2 rung, full histogram (tried_regions) mode.
	got, err := DetectAt("../../testdata/fedora-damon/proc", "../../testdata/fedora-damon/sys")
	if err != nil {
		t.Fatalf("DetectAt: %v", err)
	}
	want := Caps{Sysfs: true, Paddr: true, TriedRegions: true}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
