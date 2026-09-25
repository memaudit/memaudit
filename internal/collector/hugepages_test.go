// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"os"
	"testing"
)

func TestHugepagesCollectGolden(t *testing.T) {
	got, err := NewHugepages("../../testdata/hugepages-multi-node/sys").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/hugepages-multi-node/expected/hugepages.json", got)
}

func TestHugepagesCollectGoldenRealDualSocket(t *testing.T) {
	// Real capture from the same genuine 2-socket host as the NUMA
	// fixture (Scaleway EM-B111X-SATA). All counts are zero (no
	// hugepages configured on that box), but it's real evidence of the
	// per-node sysfs shape: two page sizes, both nodes present, and no
	// resv_hugepages at the per-node level at all, only the global one
	// — unlike hugepages-multi-node's hand-authored asymmetric-missing
	// case above, this fixture doesn't exercise the global-fallback
	// (node: -1) path, so both stay in place rather than one replacing
	// the other.
	got, err := NewHugepages("../../testdata/scw-em-b111x/sys").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/scw-em-b111x/expected/hugepages.json", got)
}

func TestHugepagesCollectNoSysfsIsNilNotError(t *testing.T) {
	// Hosts with CONFIG_HUGETLB off, or a container without
	// /sys/kernel/mm exposed, don't have a hugepages sysfs tree at all;
	// that must return an empty result, not an error. Reuses the same
	// absent-sys-root fixture the NUMA collector uses for its
	// no-NUMA-sysfs case.
	got, err := NewHugepages("../../testdata/edge-cases/vmstat-old-kernel/sys").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

// FuzzParseHugepagesDirName fuzzes the "hugepages-<N>kB" directory-name
// parser. Takes a plain string already, so no refactor was needed to
// make it fuzzable directly.
func FuzzParseHugepagesDirName(f *testing.F) {
	f.Add("hugepages-2048kB")
	f.Add("hugepages-1048576kB")
	f.Add("")
	f.Add("hugepages-kB")
	f.Add("hugepages-notanumberkB")
	f.Add("hugepages--1kB")
	f.Add("hugepages-99999999999999999999999999kB")
	f.Add("not-a-hugepages-dir")

	f.Fuzz(func(t *testing.T, name string) {
		// Must never panic, regardless of input shape.
		_, _ = parseHugepagesDirName(name)
	})
}

// FuzzParseUintFileContent fuzzes the single-line sysfs counter file
// parser shared by every hugepages field (nr_hugepages, free_hugepages,
// resv_hugepages, surplus_hugepages).
func FuzzParseUintFileContent(f *testing.F) {
	fixtures := []string{
		"../../testdata/scw-em-b111x/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages",
		"../../testdata/hugepages-multi-node/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages",
		"../../testdata/hugepages-multi-node/sys/kernel/mm/hugepages/hugepages-1048576kB/nr_hugepages",
	}
	for _, path := range fixtures {
		b, err := os.ReadFile(path) //nolint:gosec // G304: fixed test-fixture paths under testdata/, not user input
		if err != nil {
			f.Fatalf("read fixture %s: %v", path, err)
		}
		f.Add(b)
	}
	f.Add([]byte(""))
	f.Add([]byte("notanumber"))
	f.Add([]byte("  42  \n"))
	f.Add([]byte("-1"))
	f.Add([]byte("99999999999999999999999999"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic, regardless of input shape.
		_, _ = parseUintFileContent(data)
	})
}
