// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import "testing"

func TestPSICollectGolden(t *testing.T) {
	got, err := NewPSI("../../testdata/container-linux-6.12/proc").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/container-linux-6.12/expected/psi.json", got)
}

func TestPSICollectAbsentIsNilNotError(t *testing.T) {
	// CONFIG_PSI off (some hardened kernels): the collector must report
	// "nothing to emit" rather than fail the whole tick.
	got, err := NewPSI("../../testdata/edge-cases/psi-absent/proc").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}
