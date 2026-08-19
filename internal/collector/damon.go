// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"github.com/memaudit/memaudit/pkg/model"
	"github.com/memaudit/memaudit/pkg/damon"
)

// Damon collects the damon_hist record from an already-started DAMON
// session. Unlike every other collector here, it wraps live kernel state
// rather than a read-only proc/sys root — the session's lifecycle (Start
// at agent startup, Stop at shutdown) is the caller's responsibility, not
// Damon's.
type Damon struct {
	session *damon.Session
	aggrUS  uint64
}

// NewDamon returns a Damon collector reading from an already-started
// session, configured with the same aggrUS the session itself was
// started with (needed to convert Region.Age, in aggregation-interval
// units, into seconds for bucketing).
func NewDamon(session *damon.Session, aggrUS uint64) *Damon {
	return &Damon{session: session, aggrUS: aggrUS}
}

// Collect snapshots the session and buckets the result into a
// damon_hist record.
func (d *Damon) Collect() (*model.DamonHist, error) {
	regions, err := d.session.Snapshot()
	if err != nil {
		return nil, err
	}
	hist := bucketRegions(regions, d.aggrUS)
	return &hist, nil
}

// coldThresholdsUS are the cold_* bucket boundaries, in microseconds, to
// match Region.Age * aggrUS (also microseconds) without floating point.
var coldThresholdsUS = [5]uint64{
	60 * 1_000_000,        // cold_60s
	300 * 1_000_000,       // cold_300s
	3600 * 1_000_000,      // cold_1h
	6 * 3600 * 1_000_000,  // cold_6h
	24 * 3600 * 1_000_000, // cold_24h
}

// bucketRegions turns raw DAMON regions into the pre-bucketed histogram
// the product ships — never raw regions, whose cardinality is unbounded.
// A region counts toward a cold bucket only if it had zero accesses in
// the last aggregation interval; NrAccesses > 0 means "touched now,"
// regardless of Age (age resets when the access pattern changes, so a
// nonzero NrAccesses this interval means Age doesn't reflect idle time).
// Buckets are cumulative: a region cold for 24h also counts toward every
// smaller bucket.
func bucketRegions(regions []damon.Region, aggrUS uint64) model.DamonHist {
	var hist model.DamonHist
	var cold [5]uint64

	for _, r := range regions {
		sz := r.End - r.Start
		hist.MonitoredBytes += sz

		if r.NrAccesses > 0 {
			continue
		}
		ageUS := uint64(r.Age) * aggrUS
		for i, threshold := range coldThresholdsUS {
			if ageUS >= threshold {
				cold[i] += sz
			}
		}
	}

	hist.Cold60s, hist.Cold300s, hist.Cold1h, hist.Cold6h, hist.Cold24h = cold[0], cold[1], cold[2], cold[3], cold[4]
	hist.HotBytes = hist.MonitoredBytes - hist.Cold60s
	return hist
}
