// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package spool writes envelopes to JSONL, rotates completed segments to
// zstd, and enforces the on-disk spool size cap.
package spool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/memaudit/memaudit/internal/model"
	"github.com/oklog/ulid/v2"
)

const (
	defaultRotateBytes = 8 * 1024 * 1024
	defaultRotateAge   = 60 * time.Second
	defaultMaxBytes    = 2 * 1024 * 1024 * 1024
)

// Options configures a Spool. Zero values fall back to sane defaults.
type Options struct {
	// RotateBytes is the active segment size that triggers a rotation.
	// Defaults to 8 MiB.
	RotateBytes int64
	// RotateAge is the active segment age that triggers a rotation.
	// Defaults to 60s.
	RotateAge time.Duration
	// MaxBytes caps total on-disk segment size; once exceeded, the oldest
	// segments are dropped and a warning envelope is recorded. Defaults to
	// 2 GiB.
	MaxBytes int64
	// Site and Host are stamped onto the synthetic warning envelope
	// recorded when MaxBytes forces a segment to be dropped.
	Site, Host string
	// NewULID generates the ID used to name a rotated segment. Defaults to
	// a real ULID; tests can override it for determinism.
	NewULID func() string
	// Now returns the current time. Defaults to time.Now; tests override it
	// to exercise age-based rotation without sleeping.
	Now func() time.Time
}

// Spool appends envelopes to an active JSONL file and rotates it to a
// compressed segment once it grows too large or too old.
type Spool struct {
	dir         string
	rotateBytes int64
	rotateAge   time.Duration
	maxBytes    int64
	site, host  string
	newULID     func() string
	now         func() time.Time

	active      *os.File
	activeSize  int64
	activeStart time.Time
}

// Open opens (or creates) the spool directory at dir.
func Open(dir string, opts Options) (*Spool, error) {
	if opts.RotateBytes <= 0 {
		opts.RotateBytes = defaultRotateBytes
	}
	if opts.RotateAge <= 0 {
		opts.RotateAge = defaultRotateAge
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultMaxBytes
	}
	if opts.NewULID == nil {
		opts.NewULID = func() string { return ulid.Make().String() }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "active.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640) //nolint:gosec // G304: dir is operator-configured (spool.dir), not untrusted input
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	activeStart := opts.Now()
	if info.Size() > 0 {
		// Resuming an active segment left behind by a prior run: use its
		// mtime as a best-effort approximation of when it was started.
		activeStart = info.ModTime()
	}
	return &Spool{
		dir:         dir,
		rotateBytes: opts.RotateBytes,
		rotateAge:   opts.RotateAge,
		maxBytes:    opts.MaxBytes,
		site:        opts.Site,
		host:        opts.Host,
		newULID:     opts.NewULID,
		now:         opts.Now,
		active:      f,
		activeSize:  info.Size(),
		activeStart: activeStart,
	}, nil
}

// Write appends e as one JSON line to the active segment, rotating it
// first if it has grown too large or too old.
func (s *Spool) Write(e model.Envelope) error {
	if s.activeSize > 0 && s.now().Sub(s.activeStart) >= s.rotateAge {
		if err := s.rotate(); err != nil {
			return err
		}
	}

	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	n, err := s.active.Write(b)
	if err != nil {
		return err
	}
	s.activeSize += int64(n)

	if s.activeSize >= s.rotateBytes {
		return s.rotate()
	}
	return nil
}

// rotate closes the active segment, compresses it to a new
// <ulid>.jsonl.zst file, and opens a fresh, empty active segment.
func (s *Spool) rotate() error {
	activePath := filepath.Join(s.dir, "active.jsonl")

	if err := s.active.Sync(); err != nil {
		return err
	}
	if err := s.active.Close(); err != nil {
		return err
	}

	raw, err := os.ReadFile(activePath) //nolint:gosec // G304: activePath is built from the spool's own dir, not external input
	if err != nil {
		return err
	}

	segPath := filepath.Join(s.dir, fmt.Sprintf("%s.jsonl.zst", s.newULID()))
	segFile, err := os.OpenFile(segPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640) //nolint:gosec // G304: segPath is built from the spool's own dir, not external input
	if err != nil {
		return err
	}
	defer func() { _ = segFile.Close() }()

	enc, err := zstd.NewWriter(segFile)
	if err != nil {
		return err
	}
	if _, err := enc.Write(raw); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := segFile.Sync(); err != nil {
		return err
	}

	f, err := os.OpenFile(activePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640) //nolint:gosec // G304: activePath is built from the spool's own dir, not external input
	if err != nil {
		return err
	}
	s.active = f
	s.activeSize = 0
	s.activeStart = s.now()

	return s.enforceCap()
}

// enforceCap drops the oldest segments, recording a warning envelope for
// each, until total on-disk segment size is back under maxBytes.
func (s *Spool) enforceCap() error {
	for {
		segs, err := s.Segments()
		if err != nil {
			return err
		}
		if len(segs) == 0 {
			return nil
		}

		var total int64
		for _, p := range segs {
			info, err := os.Stat(p)
			if err != nil {
				return err
			}
			total += info.Size()
		}
		if total <= s.maxBytes {
			return nil
		}

		oldest := segs[0]
		if err := os.Remove(oldest); err != nil {
			return err
		}
		if err := s.writeWarning(oldest); err != nil {
			return err
		}
	}
}

// writeWarning appends a synthetic envelope to the active segment noting
// that droppedSegment was evicted to stay under the spool cap.
func (s *Spool) writeWarning(droppedSegment string) error {
	payload, err := json.Marshal(map[string]string{
		"dropped_segment": filepath.Base(droppedSegment),
		"reason":          "spool cap exceeded",
	})
	if err != nil {
		return err
	}
	b, err := json.Marshal(model.Envelope{
		TS:      s.now(),
		Site:    s.site,
		Host:    s.host,
		Type:    "spool_warning",
		Schema:  1,
		Payload: payload,
	})
	if err != nil {
		return err
	}
	b = append(b, '\n')
	n, err := s.active.Write(b)
	if err != nil {
		return err
	}
	s.activeSize += int64(n)
	return nil
}

// Close flushes any pending data to a segment and closes the spool.
func (s *Spool) Close() error {
	if s.activeSize > 0 {
		if err := s.rotate(); err != nil {
			return err
		}
	}
	return s.active.Close()
}

// Segments returns the full paths of rotated (completed) segments, oldest
// first. ULIDs sort lexicographically by creation time, so a plain name
// sort gives chronological order.
func (s *Spool) Segments() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var segs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl.zst") {
			segs = append(segs, filepath.Join(s.dir, e.Name()))
		}
	}
	sort.Strings(segs)
	return segs, nil
}
