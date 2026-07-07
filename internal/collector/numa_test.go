// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import "testing"

func TestNumaCollectGolden(t *testing.T) {
	got, err := NewNuma("../../testdata/container-linux-6.12/sys").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/container-linux-6.12/expected/numa_mem.json", got)
}

func TestNumaCollectNoNodesIsNilNotError(t *testing.T) {
	// Single-node VMs and some containers (verified against Docker
	// Desktop's own linuxkit VM) don't expose
	// /sys/devices/system/node at all; that must return an empty
	// result, not an error.
	got, err := NewNuma("../../testdata/edge-cases/vmstat-old-kernel/sys").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}
