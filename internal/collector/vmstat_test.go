// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import "testing"

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
