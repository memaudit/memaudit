// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package spool

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/memaudit/memaudit/internal/model"
)

func testEnvelope(typ string) model.Envelope {
	return model.Envelope{
		TS:      time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
		Site:    "test-site",
		Host:    "test-host",
		Type:    typ,
		Schema:  1,
		Payload: json.RawMessage(`{"foo":"bar"}`),
	}
}

func TestWriteAppendsJSONLine(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Write(testEnvelope("host_mem")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := os.Open(filepath.Join(dir, "active.jsonl"))
	if err != nil {
		t.Fatalf("open active.jsonl: %v", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var got model.Envelope
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("unmarshal line: %v", err)
	}
	if got.Type != "host_mem" {
		t.Errorf("Type = %q, want %q", got.Type, "host_mem")
	}
}

func listSegments(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var segs []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".zst" {
			segs = append(segs, e.Name())
		}
	}
	return segs
}

func decompressSegment(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}
	defer func() { _ = f.Close() }()

	dec, err := zstd.NewReader(f)
	if err != nil {
		t.Fatalf("zstd.NewReader: %v", err)
	}
	defer dec.Close()

	raw, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func TestRotatesOnSize(t *testing.T) {
	line, err := json.Marshal(testEnvelope("host_mem"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	lineSize := int64(len(line)) + 1 // + newline

	dir := t.TempDir()
	// One write alone must stay under the threshold; two must cross it.
	s, err := Open(dir, Options{RotateBytes: lineSize + lineSize/2})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Write(testEnvelope("host_mem")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if segs := listSegments(t, dir); len(segs) != 0 {
		t.Fatalf("expected no rotation after 1 write, got %v", segs)
	}
	if err := s.Write(testEnvelope("vmstat")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	segs := listSegments(t, dir)
	if len(segs) != 1 {
		t.Fatalf("expected 1 rotated segment, got %d: %v", len(segs), segs)
	}

	lines := decompressSegment(t, filepath.Join(dir, segs[0]))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines in rotated segment, got %d", len(lines))
	}

	active, err := os.ReadFile(filepath.Join(dir, "active.jsonl"))
	if err != nil {
		t.Fatalf("read active.jsonl: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected active.jsonl to be empty after rotation, got %d bytes", len(active))
	}
}

func TestRotatesOnAge(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	dir := t.TempDir()
	s, err := Open(dir, Options{
		RotateBytes: 1 << 20, // large enough that size never triggers it
		RotateAge:   60 * time.Second,
		Now:         clock,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Write(testEnvelope("host_mem")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if segs := listSegments(t, dir); len(segs) != 0 {
		t.Fatalf("expected no rotation before age threshold, got %v", segs)
	}

	now = now.Add(61 * time.Second)
	if err := s.Write(testEnvelope("vmstat")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	segs := listSegments(t, dir)
	if len(segs) != 1 {
		t.Fatalf("expected 1 rotated segment after age threshold, got %d: %v", len(segs), segs)
	}

	lines := decompressSegment(t, filepath.Join(dir, segs[0]))
	if len(lines) != 1 {
		t.Fatalf("expected rotated segment to hold the 1 pre-threshold line, got %d", len(lines))
	}

	active, err := os.ReadFile(filepath.Join(dir, "active.jsonl"))
	if err != nil {
		t.Fatalf("read active.jsonl: %v", err)
	}
	var got model.Envelope
	if err := json.Unmarshal(bytes.TrimSpace(active), &got); err != nil {
		t.Fatalf("unmarshal active.jsonl: %v", err)
	}
	if got.Type != "vmstat" {
		t.Errorf("active.jsonl Type = %q, want %q", got.Type, "vmstat")
	}
}

func TestCloseFlushesActiveSegment(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.Write(testEnvelope("host_mem")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if segs := listSegments(t, dir); len(segs) != 0 {
		t.Fatalf("expected no rotation before Close, got %v", segs)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	segs := listSegments(t, dir)
	if len(segs) != 1 {
		t.Fatalf("expected Close to flush 1 segment, got %d: %v", len(segs), segs)
	}
	lines := decompressSegment(t, filepath.Join(dir, segs[0]))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line in flushed segment, got %d", len(lines))
	}

	active, err := os.ReadFile(filepath.Join(dir, "active.jsonl"))
	if err != nil {
		t.Fatalf("read active.jsonl: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected active.jsonl empty after Close, got %d bytes", len(active))
	}
}

func TestCloseOnEmptyActiveCreatesNoSegment(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if segs := listSegments(t, dir); len(segs) != 0 {
		t.Errorf("expected no segment from closing an empty spool, got %v", segs)
	}
}

func TestSegmentsListsOldestFirst(t *testing.T) {
	dir := t.TempDir()
	var id int
	nextULID := func() string {
		id++
		return fmt.Sprintf("SEG%03d", id)
	}

	s, err := Open(dir, Options{RotateBytes: 1, NewULID: nextULID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	for _, typ := range []string{"host_mem", "vmstat", "psi"} {
		if err := s.Write(testEnvelope(typ)); err != nil {
			t.Fatalf("Write %s: %v", typ, err)
		}
	}

	segs, err := s.Segments()
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d: %v", len(segs), segs)
	}

	want := []string{"SEG001.jsonl.zst", "SEG002.jsonl.zst", "SEG003.jsonl.zst"}
	for i, w := range want {
		if filepath.Base(segs[i]) != w {
			t.Errorf("segs[%d] = %q, want basename %q", i, segs[i], w)
		}
	}
}

func TestEnforcesMaxBytesCap(t *testing.T) {
	dir := t.TempDir()

	// Phase 1: create one segment and measure its real on-disk (compressed)
	// size, since zstd output size isn't something we can predict exactly.
	s1, err := Open(dir, Options{RotateBytes: 1})
	if err != nil {
		t.Fatalf("Open (phase 1): %v", err)
	}
	if err := s1.Write(testEnvelope("host_mem")); err != nil {
		t.Fatalf("Write (phase 1): %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close (phase 1): %v", err)
	}
	segs, err := s1.Segments()
	if err != nil || len(segs) != 1 {
		t.Fatalf("expected 1 segment after phase 1, got %v (err %v)", segs, err)
	}
	info, err := os.Stat(segs[0])
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	oneSegmentSize := info.Size()

	// Phase 2: reopen with a cap that fits one segment but not two, and
	// write a second envelope, forcing a rotation that should evict the
	// first segment and leave a warning record behind.
	s2, err := Open(dir, Options{RotateBytes: 1, MaxBytes: oneSegmentSize + 10})
	if err != nil {
		t.Fatalf("Open (phase 2): %v", err)
	}
	defer func() { _ = s2.Close() }()

	if err := s2.Write(testEnvelope("vmstat")); err != nil {
		t.Fatalf("Write (phase 2): %v", err)
	}

	segsAfter, err := s2.Segments()
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segsAfter) != 1 {
		t.Fatalf("expected cap enforcement to leave exactly 1 segment, got %d: %v", len(segsAfter), segsAfter)
	}
	if segsAfter[0] == segs[0] {
		t.Errorf("expected the original (oldest) segment to be evicted, but it's still present")
	}

	active, err := os.ReadFile(filepath.Join(dir, "active.jsonl"))
	if err != nil {
		t.Fatalf("read active.jsonl: %v", err)
	}
	var warn model.Envelope
	if err := json.Unmarshal(bytes.TrimSpace(active), &warn); err != nil {
		t.Fatalf("unmarshal warning envelope: %v\ncontent: %s", err, active)
	}
	if warn.Type != "spool_warning" {
		t.Errorf("warning envelope Type = %q, want %q", warn.Type, "spool_warning")
	}
}

func TestOpenApproximatesActiveStartFromExistingFileMTime(t *testing.T) {
	dir := t.TempDir()

	// Simulate a leftover active.jsonl from a prior, ungracefully-stopped
	// run: pre-existing content, with an old mtime.
	oldLine, err := json.Marshal(testEnvelope("host_mem"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	oldLine = append(oldLine, '\n')
	activePath := filepath.Join(dir, "active.jsonl")
	if err := os.WriteFile(activePath, oldLine, 0o640); err != nil {
		t.Fatalf("seed active.jsonl: %v", err)
	}
	oldMTime := time.Date(2026, 7, 7, 11, 0, 0, 0, time.UTC)
	if err := os.Chtimes(activePath, oldMTime, oldMTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// "Now" is already 61s past the leftover file's mtime, i.e. past the
	// rotation age threshold before a single new byte has been written.
	now := oldMTime.Add(61 * time.Second)
	s, err := Open(dir, Options{
		RotateBytes: 1 << 20,
		RotateAge:   60 * time.Second,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Write(testEnvelope("vmstat")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	segs := listSegments(t, dir)
	if len(segs) != 1 {
		t.Fatalf("expected the leftover content to rotate out on the first write, got %d segments: %v", len(segs), segs)
	}
	oldLines := decompressSegment(t, filepath.Join(dir, segs[0]))
	if len(oldLines) != 1 {
		t.Fatalf("expected 1 line in the rotated leftover segment, got %d", len(oldLines))
	}

	active, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("read active.jsonl: %v", err)
	}
	var got model.Envelope
	if err := json.Unmarshal(bytes.TrimSpace(active), &got); err != nil {
		t.Fatalf("unmarshal active.jsonl: %v", err)
	}
	if got.Type != "vmstat" {
		t.Errorf("active.jsonl Type = %q, want %q (leftover content should have rotated out first)", got.Type, "vmstat")
	}
}
