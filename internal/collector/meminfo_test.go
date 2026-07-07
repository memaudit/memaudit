// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import "testing"

func TestMeminfoCollectGolden(t *testing.T) {
	got, err := NewMeminfo("../../testdata/container-linux-6.12/proc").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/container-linux-6.12/expected/host_mem.json", got)
}
