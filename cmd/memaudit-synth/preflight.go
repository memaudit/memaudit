// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

const (
	// minHeadroomBytes is the minimum RAM a box needs beyond --alloc for
	// the OS itself to keep running once the full allocation is touched
	// and made resident — independent of the band check below; this is
	// pure OOM-avoidance.
	minHeadroomBytes = 1 * gib

	// maxAmbientRatio bounds how much non-workload ("ambient") RAM a box
	// may have, as a multiple of allocBytes (1 == ambient capped at
	// allocBytes itself). Past this point the run would mostly measure
	// ambient noise rather than the synthetic workload, even though
	// bandBounds' high bound technically still admits it (it's now
	// ambient-aware — see band.go) — this check exists so a large/busy
	// box gets a clear "this isn't a meaningful test" message instead of
	// a very wide, uninformative band. It's a deliberate tradeoff, not a
	// tightenable knob: real ambient RAM is real DAMON-visible noise, so
	// admitting it is required for correctness (see band.go), and no
	// value of this ratio can make the band both tolerant of real ambient
	// noise and maximally sensitive to a broken measurement — that
	// precision would need scoping DAMON to just this tool's own memory,
	// which was deliberately ruled out (see README) as too complex for
	// what's meant to be a simple, auditable tool.
	maxAmbientRatio = 1
)

// checkPreflight reports whether totalBytes (the box's total RAM) is
// suitable for a run with allocBytes: enough headroom above allocBytes
// that touching the whole allocation doesn't risk OOM, and not so much
// more that the run stops meaningfully testing the workload versus
// ambient noise.
func checkPreflight(totalBytes, allocBytes uint64) error {
	if totalBytes < allocBytes+minHeadroomBytes {
		return fmt.Errorf("total RAM (%d bytes) leaves less than %d bytes of headroom over --alloc (%d bytes): touching the full allocation risks OOM", totalBytes, uint64(minHeadroomBytes), allocBytes)
	}
	ambient := totalBytes - allocBytes
	maxAmbient := maxAmbientRatio * allocBytes
	if ambient > maxAmbient {
		return fmt.Errorf("total RAM (%d bytes) has %d bytes beyond --alloc (%d bytes), more than %dx --alloc: the run would mostly measure ambient noise instead of the workload; run on a smaller/quieter box or pass --force", totalBytes, ambient, allocBytes, maxAmbientRatio)
	}
	return nil
}
