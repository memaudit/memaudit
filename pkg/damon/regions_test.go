// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// writeFixtureFile writes value into a file at dir/parts..., creating
// parent directories as needed.
func writeFixtureFile(t *testing.T, dir string, parts []string, value string) {
	t.Helper()
	full := filepath.Join(append([]string{dir}, parts...)...)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(value), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestSnapshotTriggersUpdateThenReadsRegions(t *testing.T) {
	kdamond := t.TempDir()
	tr := []string{"contexts", "0", "schemes", "0", "tried_regions"}

	writeFixtureFile(t, kdamond, append(tr, "total_bytes"), "3000")
	writeFixtureFile(t, kdamond, append(tr, "0", "start"), "1000")
	writeFixtureFile(t, kdamond, append(tr, "0", "end"), "3000")
	writeFixtureFile(t, kdamond, append(tr, "0", "nr_accesses"), "5")
	writeFixtureFile(t, kdamond, append(tr, "0", "age"), "12")
	writeFixtureFile(t, kdamond, append(tr, "1", "start"), "3000")
	writeFixtureFile(t, kdamond, append(tr, "1", "end"), "4000")
	writeFixtureFile(t, kdamond, append(tr, "1", "nr_accesses"), "0")
	writeFixtureFile(t, kdamond, append(tr, "1", "age"), "3500")

	var triggered []recordedWrite
	sess := &Session{kdamond: kdamond, writeFile: recordingWriter(&triggered)}

	got, err := sess.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	wantTrigger := []recordedWrite{{filepath.Join(kdamond, "state"), "update_schemes_tried_regions"}}
	if !reflect.DeepEqual(triggered, wantTrigger) {
		t.Fatalf("trigger write = %#v, want %#v", triggered, wantTrigger)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].Start < got[j].Start })
	want := []Region{
		{Start: 1000, End: 3000, NrAccesses: 5, Age: 12},
		{Start: 3000, End: 4000, NrAccesses: 0, Age: 3500},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSnapshotNoRegionsIsEmptyNotError(t *testing.T) {
	kdamond := t.TempDir()
	writeFixtureFile(t, kdamond, []string{"contexts", "0", "schemes", "0", "tried_regions", "total_bytes"}, "0")

	sess := &Session{kdamond: kdamond, writeFile: func(string, string) error { return nil }}
	got, err := sess.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}
