// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"testing"
	"time"

	"github.com/memaudit/memaudit/internal/config"
)

func fixedNow() time.Time { return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC) }

func TestBuildSourcesCgroupDisabled(t *testing.T) {
	cfg := config.CollectorsConfig{} // Cgroup.Enabled defaults to false (zero value)
	srcs := buildSources(cfg, "../../testdata/container-linux-6.12/proc", "../../testdata/container-linux-6.12/sys", "acme", "host-a", nil, fixedNow)

	got := typesOf(srcs)
	want := []string{"host_mem", "vmstat", "psi", "numa_mem", "hugepages"}
	assertSameSet(t, got, want)

	for _, src := range srcs {
		envs, err := src.collect()
		if err != nil {
			t.Fatalf("%s collect: %v", src.typ, err)
		}
		switch src.typ {
		case "host_mem", "vmstat", "psi", "numa_mem":
			if len(envs) == 0 {
				t.Errorf("%s: got 0 envelopes against a fixture with real data", src.typ)
			}
		}
		for _, e := range envs {
			if e.Site != "acme" || e.Host != "host-a" || e.Type != src.typ || e.Schema != 1 {
				t.Errorf("%s: bad envelope %+v", src.typ, e)
			}
			if !e.TS.Equal(fixedNow()) {
				t.Errorf("%s: TS = %v, want injected now()", src.typ, e.TS)
			}
		}
	}
}

func TestBuildSourcesCgroupEnabled(t *testing.T) {
	cfg := config.CollectorsConfig{
		Cgroup: config.CgroupConfig{
			Enabled: true,
			Globs:   []string{"system.slice/*.service", "kubepods.slice/**"},
			Max:     500,
		},
	}
	srcs := buildSources(cfg, "../../testdata/container-linux-6.12/proc", "../../testdata/cgroup-v2-k8s/sys", "acme", "host-a", nil, fixedNow)

	got := typesOf(srcs)
	want := []string{"host_mem", "vmstat", "psi", "numa_mem", "hugepages", "cgroup_mem"}
	assertSameSet(t, got, want)

	for _, src := range srcs {
		if src.typ != "cgroup_mem" {
			continue
		}
		envs, err := src.collect()
		if err != nil {
			t.Fatalf("cgroup_mem collect: %v", err)
		}
		if len(envs) == 0 {
			t.Fatal("cgroup_mem: got 0 envelopes against a fixture with real cgroups")
		}
	}
}

func TestBuildSourcesDamonDisabledByDefault(t *testing.T) {
	cfg := config.CollectorsConfig{} // Damon.Enabled defaults to false (zero value)
	srcs := buildSources(cfg, "../../testdata/fedora-damon/proc", "../../testdata/fedora-damon/sys", "acme", "host-a", nil, fixedNow)

	for _, src := range srcs {
		if src.typ == "damon_hist" {
			t.Fatal("damon_hist source registered despite Damon.Enabled=false")
		}
	}
}

func TestBuildSourcesDamonAbsentSkipsGracefully(t *testing.T) {
	cfg := config.CollectorsConfig{
		Damon: config.DamonConfig{Enabled: true, SampleUS: 5_000, AggrUS: 100_000, MaxRegions: 1_000},
	}
	// No DAMON sysfs on this fixture root at all: buildSources must not
	// panic or error, just skip registering damon_hist.
	srcs := buildSources(cfg, "../../testdata/edge-cases/damon-absent/proc", "../../testdata/edge-cases/vmstat-old-kernel/sys", "acme", "host-a", nil, fixedNow)

	for _, src := range srcs {
		if src.typ == "damon_hist" {
			t.Fatal("damon_hist source registered despite no DAMON sysfs on this host")
		}
	}
	// Every other source should still be there — DAMON's absence must not
	// take anything else down with it.
	got := typesOf(srcs)
	want := []string{"host_mem", "vmstat", "psi", "numa_mem", "hugepages"}
	assertSameSet(t, got, want)
}

func typesOf(srcs []source) []string {
	out := make([]string, len(srcs))
	for i, s := range srcs {
		out[i] = s.typ
	}
	return out
}

func assertSameSet(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("got %v, want %v (missing %q)", got, want, w)
		}
	}
}

// TestEnvelopesSharedTimestamp verifies that all envelopes from a single
// collect() call share the same timestamp, even when collect() produces
// multiple records. This is a regression test for a bug where now() was
// called inside the envelope-marshaling loop instead of once per batch,
// causing multi-record sources to have inconsistent timestamps.
func TestEnvelopesSharedTimestamp(t *testing.T) {
	// Create a counter-based clock that returns a distinct time per call.
	// If now() is called once per batch, all envelopes get callCount=0.
	// If now() is called once per record, envelopes get distinct callCounts.
	var callCount int
	counterClock := func() time.Time {
		defer func() { callCount++ }()
		return time.Date(2026, 7, 29, 12, 0, callCount, 0, time.UTC)
	}

	// Test via multi-record source (numa_mem produces multiple records).
	// We use the testdata fixture which should produce multiple numa nodes.
	cfg := config.CollectorsConfig{}
	srcs := buildSources(cfg, "../../testdata/container-linux-6.12/proc", "../../testdata/container-linux-6.12/sys", "acme", "host-a", nil, counterClock)

	for _, src := range srcs {
		if src.typ != "numa_mem" {
			continue
		}
		callCount = 0 // reset counter before collecting
		envs, err := src.collect()
		if err != nil {
			t.Fatalf("numa_mem collect: %v", err)
		}
		if len(envs) < 2 {
			t.Skip("numa_mem fixture has < 2 records; skipping regression test")
		}

		// All envelopes should share the same TS (the TS from the first
		// now() call at the start of the batch, not per-record calls).
		firstTS := envs[0].TS
		for i, e := range envs {
			if !e.TS.Equal(firstTS) {
				t.Errorf("envelope %d has TS %v, want %v (all should share one tick timestamp)", i, e.TS, firstTS)
			}
		}
		return // test passed, return after testing numa_mem
	}
	t.Fatal("numa_mem source not found")
}
