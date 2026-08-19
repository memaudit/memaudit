// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package spool

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/memaudit/memaudit/pkg/model"
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

// listTempFiles returns the in-progress segment files (the ones a
// rotation writes before publishing them under their final name).
func listTempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var tmps []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			tmps = append(tmps, e.Name())
		}
	}
	return tmps
}

// TestRotatePublishesSegmentOnlyWhenComplete pins the invariant a
// concurrent reader depends on: a segment appears under its final
// ".jsonl.zst" name only once it is fully written and synced. A shipper
// runs alongside the writer, lists segments, POSTs them and then deletes
// them — so a file carrying its final name while still being written
// would be shipped half-empty and unlinked out from under the rotation
// that is still filling it.
//
// The assertions run at the one instant that matters: after the segment
// bytes are on disk, before the rename that publishes them. Writing
// straight to the final name (as this code used to) fails every one of
// them.
func TestRotatePublishesSegmentOnlyWhenComplete(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{RotateBytes: 1, NewULID: func() string { return "SEG001" }})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	segPath := filepath.Join(dir, "SEG001.jsonl.zst")

	var hookCalls int
	s.beforePublish = func(tmpPath, gotSegPath string) {
		hookCalls++

		if gotSegPath != segPath {
			t.Errorf("segment path = %q, want %q", gotSegPath, segPath)
		}
		info, err := os.Stat(tmpPath)
		if err != nil {
			t.Errorf("stat in-progress segment %q: %v", tmpPath, err)
		} else if info.Size() == 0 {
			t.Errorf("in-progress segment %q is empty at publish time", tmpPath)
		}

		if _, err := os.Stat(segPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("final segment name %q already exists before the publishing rename (stat err = %v)", segPath, err)
		}
		segs, err := s.Segments()
		if err != nil {
			t.Errorf("Segments during rotation: %v", err)
		}
		if len(segs) != 0 {
			t.Errorf("Segments exposed %v mid-rotation; a concurrent shipper could ship and delete a segment still being written", segs)
		}
	}

	if err := s.Write(testEnvelope("host_mem")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if hookCalls != 1 {
		t.Fatalf("expected exactly 1 rotation, got %d", hookCalls)
	}

	segs, err := s.Segments()
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) != 1 || segs[0] != segPath {
		t.Fatalf("after rotation, Segments = %v, want [%s]", segs, segPath)
	}
	if lines := decompressSegment(t, segPath); len(lines) != 1 {
		t.Errorf("expected 1 line in the published segment, got %d", len(lines))
	}
	if tmps := listTempFiles(t, dir); len(tmps) != 0 {
		t.Errorf("rotation left in-progress files behind: %v", tmps)
	}
}

// TestOpenClearsAbandonedInProgressSegment covers the crash case: a
// rotation that died between creating its temporary segment and
// publishing it leaves a file Segments() can't see and the cap can't
// count, so the next Open has to clear it out.
func TestOpenClearsAbandonedInProgressSegment(t *testing.T) {
	dir := t.TempDir()
	orphan := filepath.Join(dir, "SEG001.jsonl.zst.tmp")
	if err := os.WriteFile(orphan, []byte("half a zstd frame"), 0o640); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	if tmps := listTempFiles(t, dir); len(tmps) != 0 {
		t.Errorf("Open left abandoned in-progress segments behind: %v", tmps)
	}
}

// checkSegmentComplete reports whether path holds a whole, readable
// segment: a complete zstd frame decoding to at least one newline-
// terminated JSON line. A file that a rotation is still streaming into
// fails here — as an empty read, a truncated zstd frame, or a partial
// trailing line.
func checkSegmentComplete(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	dec, err := zstd.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("zstd.NewReader: %w", err)
	}
	defer dec.Close()

	body, err := io.ReadAll(dec)
	if err != nil {
		return fmt.Errorf("decompress (%d compressed bytes): %w", len(raw), err)
	}
	if len(body) == 0 {
		return fmt.Errorf("segment decoded to 0 bytes (%d compressed bytes)", len(raw))
	}
	if body[len(body)-1] != '\n' {
		return errors.New("segment does not end on a line boundary")
	}
	for i, line := range bytes.Split(bytes.TrimSuffix(body, []byte{'\n'}), []byte{'\n'}) {
		var e model.Envelope
		if err := json.Unmarshal(line, &e); err != nil {
			return fmt.Errorf("line %d is not a valid envelope: %w", i, err)
		}
	}
	return nil
}

// TestSegmentsNeverExposesPartialSegment reproduces what the agent does
// in push mode — collectors rotating segments while the shipper
// concurrently lists and reads them — and asserts that every path
// Segments hands out is already complete. Against a rotation that streams
// into the final name, the reader below catches empty or truncated
// segments: it validates each path the first time it sees it, and that
// first sighting lands inside the compress-and-sync window.
func TestSegmentsNeverExposesPartialSegment(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{RotateBytes: 512 * 1024})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Bulky, poorly-compressible payloads keep each rotation busy long
	// enough for a concurrent reader to sample the window.
	rng := rand.New(rand.NewPCG(1, 2))
	blob := make([]byte, 16*1024)
	for i := range blob {
		blob[i] = byte(rng.UintN(256))
	}
	env := testEnvelope("host_mem")
	env.Payload = json.RawMessage(`{"blob":"` + hex.EncodeToString(blob) + `"}`)

	stop := make(chan struct{})
	errCh := make(chan error, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		seen := map[string]bool{}
		for {
			select {
			case <-stop:
				return
			default:
			}
			segs, err := s.Segments()
			if err != nil {
				errCh <- fmt.Errorf("Segments: %w", err)
				return
			}
			for _, p := range segs {
				if seen[p] {
					continue
				}
				seen[p] = true
				if strings.HasSuffix(p, ".tmp") {
					errCh <- fmt.Errorf("Segments returned an in-progress file: %s", p)
					continue
				}
				if err := checkSegmentComplete(p); err != nil {
					errCh <- fmt.Errorf("segment %s was listed before it was complete: %w", filepath.Base(p), err)
				}
			}
		}
	}()

	const rotations = 6
	for i := 0; i < rotations*(512*1024/len(env.Payload)+1); i++ {
		if err := s.Write(env); err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	close(stop)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if tmps := listTempFiles(t, dir); len(tmps) != 0 {
		t.Errorf("in-progress files left behind: %v", tmps)
	}
	segs, err := s.Segments()
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) < rotations {
		t.Fatalf("expected at least %d rotations to have happened, got %d segments", rotations, len(segs))
	}
	for _, p := range segs {
		if err := checkSegmentComplete(p); err != nil {
			t.Errorf("final segment %s: %v", filepath.Base(p), err)
		}
	}
}
