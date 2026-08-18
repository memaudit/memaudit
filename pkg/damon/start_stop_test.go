// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"errors"
	"reflect"
	"testing"
)

var errBoom = errors.New("boom")

type recordedWrite struct {
	path  string
	value string
}

func recordingWriter(rec *[]recordedWrite) sysfsWriteFunc {
	return func(path, value string) error {
		*rec = append(*rec, recordedWrite{path, value})
		return nil
	}
}

func testConfig() Config {
	return Config{
		Ops:        "paddr",
		SampleUS:   5_000,
		AggrUS:     100_000,
		UpdateUS:   1_000_000,
		MinRegions: 10,
		MaxRegions: 1_000,
		Regions: []AddrRange{
			{Start: 0x1000, End: 0x9fbff},    // inclusive per /proc/iomem
			{Start: 0x100000, End: 0x7ffdbfff},
		},
	}
}

func TestStartWritesExactSequence(t *testing.T) {
	const admin = "/sys/kernel/mm/damon/admin"
	const k = admin + "/kdamonds/0"
	const c = k + "/contexts/0"
	const maxU64 = "18446744073709551615"

	var rec []recordedWrite
	_, err := start("/sys", testConfig(), recordingWriter(&rec))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	want := []recordedWrite{
		{k + "/state", "off"}, // best-effort: stop any leftover session before reconfiguring
		{admin + "/kdamonds/nr_kdamonds", "1"},
		{k + "/contexts/nr_contexts", "1"},
		{c + "/operations", "paddr"},
		{c + "/monitoring_attrs/intervals/sample_us", "5000"},
		{c + "/monitoring_attrs/intervals/aggr_us", "100000"},
		{c + "/monitoring_attrs/intervals/update_us", "1000000"},
		{c + "/monitoring_attrs/nr_regions/min", "10"},
		{c + "/monitoring_attrs/nr_regions/max", "1000"},
		{c + "/targets/nr_targets", "1"},
		{c + "/targets/0/regions/nr_regions", "2"},
		// AddrRange.End is inclusive (matches /proc/iomem); DAMON's sysfs
		// region end is exclusive, so this must be End+1.
		{c + "/targets/0/regions/0/start", "4096"},
		{c + "/targets/0/regions/0/end", "654336"},
		{c + "/targets/0/regions/1/start", "1048576"},
		{c + "/targets/0/regions/1/end", "2147336192"},
		{c + "/schemes/nr_schemes", "1"},
		{c + "/schemes/0/action", "stat"},
		{c + "/schemes/0/access_pattern/sz/min", "0"},
		{c + "/schemes/0/access_pattern/sz/max", maxU64},
		{c + "/schemes/0/access_pattern/nr_accesses/min", "0"},
		{c + "/schemes/0/access_pattern/nr_accesses/max", maxU64},
		{c + "/schemes/0/access_pattern/age/min", "0"},
		{c + "/schemes/0/access_pattern/age/max", maxU64},
		{k + "/state", "on"},
	}
	if !reflect.DeepEqual(rec, want) {
		t.Fatalf("write sequence mismatch\ngot:  %#v\nwant: %#v", rec, want)
	}
}

func TestStartAbortsOnFirstWriteError(t *testing.T) {
	failAt := "/sys/kernel/mm/damon/admin/kdamonds/0/contexts/0/operations"
	writeFn := func(path, value string) error {
		if path == failAt {
			return errBoom
		}
		return nil
	}
	_, err := start("/sys", testConfig(), writeFn)
	if err == nil {
		t.Fatal("start: got nil error, want the write failure surfaced")
	}
}

func TestStopWritesStateOff(t *testing.T) {
	var rec []recordedWrite
	sess, err := start("/sys", testConfig(), recordingWriter(&rec))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	rec = nil // only care about Stop's own write

	if err := sess.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want := []recordedWrite{{"/sys/kernel/mm/damon/admin/kdamonds/0/state", "off"}}
	if !reflect.DeepEqual(rec, want) {
		t.Fatalf("got %#v, want %#v", rec, want)
	}
}
