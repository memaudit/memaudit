// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestBandBoundsZeroAmbientMatchesReferenceCase(t *testing.T) {
	// totalBytes == allocBytes (zero ambient RAM beyond the workload) is
	// the idealized reference point the band's slack constants were
	// picked against: an 8GiB/2GiB run should land exactly on the
	// original 5.0-6.5GiB target.
	low, high, err := bandBounds(8*gib, 2*gib, 8*gib)
	if err != nil {
		t.Fatalf("bandBounds: unexpected error: %v", err)
	}
	wantLow := uint64(5 * gib)
	wantHigh := uint64(6*gib + gib/2)
	if low != wantLow || high != wantHigh {
		t.Errorf("bandBounds(8GiB, 2GiB, 8GiB) = [%d, %d], want [%d, %d]", low, high, wantLow, wantHigh)
	}
}

func TestBandBoundsAddsAmbientToHighBound(t *testing.T) {
	// A box with real headroom (totalBytes > allocBytes) also has that
	// extra RAM monitored by DAMON — the high bound must widen to admit
	// it honestly instead of assuming a fixed, box-independent slack.
	ambient := uint64(3 * gib / 4) // 0.75GiB of non-workload RAM
	low, high, err := bandBounds(8*gib, 2*gib, 8*gib+ambient)
	if err != nil {
		t.Fatalf("bandBounds: unexpected error: %v", err)
	}
	wantLow := uint64(5 * gib)
	wantHigh := uint64(6*gib + ambient + gib/2)
	if low != wantLow || high != wantHigh {
		t.Errorf("bandBounds with ambient = [%d, %d], want [%d, %d]", low, high, wantLow, wantHigh)
	}
}

func TestBandBoundsClampsLowAtZero(t *testing.T) {
	// cold = alloc-hot = 0.5GiB, which is less than the 1GiB the formula
	// would otherwise subtract — low must clamp to 0, not underflow.
	low, high, err := bandBounds(gib+gib/2, gib, gib+gib/2)
	if err != nil {
		t.Fatalf("bandBounds: unexpected error: %v", err)
	}
	if low != 0 {
		t.Errorf("bandBounds low = %d, want 0 (clamped)", low)
	}
	wantHigh := uint64(gib/2 + gib/2)
	if high != wantHigh {
		t.Errorf("bandBounds high = %d, want %d", high, wantHigh)
	}
}

func TestBandBoundsRejectsHotExceedingAlloc(t *testing.T) {
	if _, _, err := bandBounds(2*gib, 8*gib, 2*gib); err == nil {
		t.Error("bandBounds(2GiB, 8GiB, 2GiB): expected error when hot > alloc, got nil")
	}
}

func TestBandBoundsRejectsTotalBelowAlloc(t *testing.T) {
	if _, _, err := bandBounds(8*gib, 2*gib, 7*gib); err == nil {
		t.Error("bandBounds(8GiB, 2GiB, 7GiB): expected error when totalBytes < allocBytes, got nil")
	}
}
