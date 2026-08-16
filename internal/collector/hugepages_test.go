// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import "testing"

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
