// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"testing"

	"github.com/memaudit/memaudit/pkg/damon"
	"github.com/memaudit/memaudit/pkg/model"
)

// aggrUS below is 100_000 (100ms), the collector's default aggregation
// interval: cold threshold 300s at 100ms aggr -> age >= 3000.

func TestBucketRegionsAllHotIsZeroCold(t *testing.T) {
	regions := []damon.Region{
		{Start: 0, End: 1000, NrAccesses: 5, Age: 50_000}, // accessed this interval: never cold, regardless of age
	}
	got := bucketRegions(regions, 100_000)
	want := model.DamonHist{MonitoredBytes: 1000, HotBytes: 1000}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBucketRegionsColdBucketsAreCumulative(t *testing.T) {
	// age*aggrUS = 3000*100_000us = 300_000_000us = 300s: qualifies for
	// cold_60s and cold_300s, not cold_1h (3600s).
	regions := []damon.Region{
		{Start: 0, End: 2000, NrAccesses: 0, Age: 3000},
	}
	got := bucketRegions(regions, 100_000)
	want := model.DamonHist{
		MonitoredBytes: 2000,
		HotBytes:       0,
		Cold60s:        2000,
		Cold300s:       2000,
		Cold1h:         0,
		Cold6h:         0,
		Cold24h:        0,
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBucketRegionsMixedAges(t *testing.T) {
	regions := []damon.Region{
		{Start: 0, End: 100, NrAccesses: 3, Age: 999_999},   // hot: accessed this interval
		{Start: 100, End: 300, NrAccesses: 0, Age: 599},     // 59.9s: below even cold_60s
		{Start: 300, End: 700, NrAccesses: 0, Age: 864_000}, // 24h exactly: qualifies for every bucket
	}
	got := bucketRegions(regions, 100_000)
	want := model.DamonHist{
		MonitoredBytes: 700,
		// HotBytes = MonitoredBytes - Cold60s: the 100-300 region isn't
		// cold enough for cold_60s (59.9s < 60s) even though NrAccesses
		// is 0 this interval, so it counts as "hot" under this
		// definition alongside the genuinely-just-accessed 0-100 region.
		HotBytes: 300,
		Cold60s:  400, // only 300-700 qualifies (100-300's age is too low)
		Cold300s: 400,
		Cold1h:   400,
		Cold6h:   400,
		Cold24h:  400,
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBucketRegionsEmpty(t *testing.T) {
	got := bucketRegions(nil, 100_000)
	want := model.DamonHist{}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
