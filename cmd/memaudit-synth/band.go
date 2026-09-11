// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

const gib = 1024 * 1024 * 1024

const (
	// bandLowSlackBytes and bandHighSlackBytes are DAMON's own
	// measurement noise (region-granularity effects near the hot/cold
	// boundary) — not ambient RAM, which bandBounds accounts for
	// separately via totalBytes.
	bandLowSlackBytes  = 1 * gib     // 1 GiB
	bandHighSlackBytes = 1 * gib / 2 // 0.5 GiB
)

// bandBounds computes the expected cold_300s byte range for a run with the
// given alloc/hot sizes on a box with totalBytes of RAM. DAMON monitors the
// whole machine's physical memory, not just this tool's allocation, so any
// RAM beyond allocBytes ("ambient": OS, caches, other processes, or simply
// free/unused pages) also ages into cold_300s — the high bound must admit
// it honestly rather than assume a fixed, box-independent slack. At
// totalBytes == allocBytes (zero ambient) the band collapses to the
// original 5.0-6.5GiB reference case for the 8GiB/2GiB default.
func bandBounds(allocBytes, hotBytes, totalBytes uint64) (low, high uint64, err error) {
	if hotBytes > allocBytes {
		return 0, 0, fmt.Errorf("hot size (%d bytes) exceeds alloc size (%d bytes)", hotBytes, allocBytes)
	}
	if totalBytes < allocBytes {
		return 0, 0, fmt.Errorf("total RAM (%d bytes) is less than alloc size (%d bytes)", totalBytes, allocBytes)
	}
	cold := allocBytes - hotBytes
	ambient := totalBytes - allocBytes

	if cold > bandLowSlackBytes {
		low = cold - bandLowSlackBytes
	}
	high = cold + ambient + bandHighSlackBytes

	return low, high, nil
}
