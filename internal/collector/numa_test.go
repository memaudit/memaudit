// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"bytes"
	"os"
	"strconv"
	"testing"
)

func TestNumaCollectGolden(t *testing.T) {
	got, err := NewNuma("../../testdata/container-linux-6.12/sys").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/container-linux-6.12/expected/numa_mem.json", got)
}

// FuzzScanNodeMeminfo fuzzes the "Node N Key:  value [unit]" line
// format used by .../nodeN/meminfo, plus the ParseUint step every
// caller applies to the extracted value (mirroring collectNode's own
// apply closure, without needing the model.NumaMem setter machinery).
func FuzzScanNodeMeminfo(f *testing.F) {
	fixtures := []string{
		"../../testdata/container-linux-6.12/sys/devices/system/node/node0/meminfo",
		"../../testdata/container-linux-6.12/sys/devices/system/node/node1/meminfo",
		"../../testdata/scw-em-b111x/sys/devices/system/node/node0/meminfo",
	}
	for _, path := range fixtures {
		b, err := os.ReadFile(path) //nolint:gosec // G304: fixed test-fixture paths under testdata/, not user input
		if err != nil {
			f.Fatalf("read fixture %s: %v", path, err)
		}
		f.Add(b)
	}
	f.Add([]byte(""))
	f.Add([]byte("Node 0 MemTotal:"))
	f.Add([]byte("Node 0 MemTotal:       notanumber kB"))
	f.Add([]byte("Node notanumber MemTotal:       12345 kB"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic, regardless of input shape.
		_ = scanReader(bytes.NewReader(data), func(fields []string) error {
			_, val, ok := nodeMeminfoKV(fields)
			if !ok {
				return nil
			}
			_, _ = strconv.ParseUint(val, 10, 64)
			return nil
		})
	})
}

// FuzzScanKV fuzzes the plain "key value" line format shared by
// .../nodeN/numastat and cgroup.go's memory.stat (both call scanKV),
// plus the ParseUint step both callers apply to the extracted value.
func FuzzScanKV(f *testing.F) {
	fixtures := []string{
		"../../testdata/container-linux-6.12/sys/devices/system/node/node0/numastat",
		"../../testdata/scw-em-b111x/sys/devices/system/node/node0/numastat",
		"../../testdata/cgroup-v2-k8s/sys/fs/cgroup/kubepods.slice/memory.stat",
	}
	for _, path := range fixtures {
		b, err := os.ReadFile(path) //nolint:gosec // G304: fixed test-fixture paths under testdata/, not user input
		if err != nil {
			f.Fatalf("read fixture %s: %v", path, err)
		}
		f.Add(b)
	}
	f.Add([]byte(""))
	f.Add([]byte("numa_hit"))
	f.Add([]byte("numa_hit notanumber"))
	f.Add([]byte("numa_hit 99999999999999999999999999"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic, regardless of input shape.
		_ = scanReader(bytes.NewReader(data), func(fields []string) error {
			_, val, ok := plainKV(fields)
			if !ok {
				return nil
			}
			_, _ = strconv.ParseUint(val, 10, 64)
			return nil
		})
	})
}

func TestNumaCollectGoldenRealDualSocket(t *testing.T) {
	// Real capture from a genuine 2-socket host (Scaleway EM-B111X-SATA,
	// 2x Intel Xeon E5-2620), replacing the hand-authored NUMA sysfs in
	// container-linux-6.12 with actual multi-node hardware data.
	got, err := NewNuma("../../testdata/scw-em-b111x/sys").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/scw-em-b111x/expected/numa_mem.json", got)
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
