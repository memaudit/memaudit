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
