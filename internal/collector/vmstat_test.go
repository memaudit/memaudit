// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"bytes"
	"os"
	"testing"
)

func TestVmstatCollectGolden(t *testing.T) {
	cases := []struct {
		name string
		root string
		want string
	}{
		{
			name: "container-linux-6.12",
			root: "../../testdata/container-linux-6.12/proc",
			want: "../../testdata/container-linux-6.12/expected/vmstat.json",
		},
		{
			// Pre-5.8 kernels report combined workingset_refault /
			// workingset_activate counters instead of the _anon/_file
			// split memaudit stores; those two unsplit keys must be
			// ignored (left zero), not cause an error.
			name: "old-kernel-missing-refault-split",
			root: "../../testdata/edge-cases/vmstat-old-kernel/proc",
			want: "../../testdata/edge-cases/vmstat-old-kernel/expected/vmstat.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewVmstat(tc.root).Collect()
			if err != nil {
				t.Fatalf("Collect: %v", err)
			}
			assertGoldenJSON(t, tc.want, got)
		})
	}
}

// FuzzParseVmstat fuzzes the hand-rolled /proc/vmstat scanner - there's
// no prometheus/procfs API for this file, so memaudit parses all of it
// itself. Seeded from real fixtures, including a pre-5.8-kernel capture
// with the older combined refault/activate counter names.
func FuzzParseVmstat(f *testing.F) {
	fixtures := []string{
		"../../testdata/container-linux-6.12/proc/vmstat",
		"../../testdata/scw-em-b111x/proc/vmstat",
		"../../testdata/edge-cases/vmstat-old-kernel/proc/vmstat",
	}
	for _, path := range fixtures {
		b, err := os.ReadFile(path) //nolint:gosec // G304: fixed test-fixture paths under testdata/, not user input
		if err != nil {
			f.Fatalf("read fixture %s: %v", path, err)
		}
		f.Add(b)
	}
	f.Add([]byte(""))
	f.Add([]byte("pgscan_kswapd"))
	f.Add([]byte("pgscan_kswapd notanumber"))
	f.Add([]byte("pgscan_kswapd 99999999999999999999999999"))
	f.Add([]byte("pgscan_kswapd 1 extra fields here"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic, regardless of input shape.
		_, _ = parseVmstat(bytes.NewReader(data))
	})
}
