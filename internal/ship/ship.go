// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package ship drains spool segments to memaudit-ingest with retry and
// backoff. It has no knowledge of air-gapped bundle mode: a caller in that
// mode simply never invokes Run.
package ship

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultMinBackoff = 1 * time.Second
	defaultMaxBackoff = 5 * time.Minute
)

// Config configures a Shipper.
type Config struct {
	// URL is the ingest batch endpoint, e.g. https://ingest.example/v1/batch.
	URL string
	// Token is the site's bearer token.
	Token string
	// Client is the HTTP client used to ship. Defaults to http.DefaultClient.
	Client *http.Client
	// Sleep waits for d, returning early with ctx.Err() if ctx is
	// cancelled first. Defaults to a real timer; tests override it to
	// exercise backoff without actually waiting.
	Sleep func(ctx context.Context, d time.Duration) error
	// MinBackoff and MaxBackoff bound the shared, exponential backoff
	// applied across all retryable failures. Default to 1s and 5m.
	MinBackoff, MaxBackoff time.Duration
}

// Shipper sends spooled segments to an ingest endpoint, retrying
// retryable failures with a single shared, exponential backoff.
type Shipper struct {
	url        string
	token      string
	client     *http.Client
	sleep      func(ctx context.Context, d time.Duration) error
	minBackoff time.Duration
	maxBackoff time.Duration

	backoff time.Duration
}

// New creates a Shipper.
func New(cfg Config) *Shipper {
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	if cfg.Sleep == nil {
		cfg.Sleep = realSleep
	}
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = defaultMinBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultMaxBackoff
	}
	return &Shipper{
		url:        cfg.URL,
		token:      cfg.Token,
		client:     cfg.Client,
		sleep:      cfg.Sleep,
		minBackoff: cfg.MinBackoff,
		maxBackoff: cfg.MaxBackoff,
		backoff:    cfg.MinBackoff,
	}
}

func realSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Run lists segments via list and ships each in order. A retryable
// failure (network error or 5xx) is retried against the same segment
// after a shared backoff; a permanent failure (4xx) drops the segment and
// moves on. Run returns when list reports no more segments, or when ctx
// is cancelled.
func (s *Shipper) Run(ctx context.Context, list func() ([]string, error)) error {
	for {
		segs, err := list()
		if err != nil {
			return err
		}
		if len(segs) == 0 {
			return nil
		}

		for _, path := range segs {
			if err := s.shipWithRetry(ctx, path); err != nil {
				return err
			}
		}
	}
}

// shipWithRetry ships path, retrying on retryable failures until it
// succeeds, permanently fails, or ctx is cancelled.
func (s *Shipper) shipWithRetry(ctx context.Context, path string) error {
	for {
		retry, err := s.ship(ctx, path)
		if err == nil {
			s.backoff = s.minBackoff
			return nil
		}
		if !retry {
			slog.Error("dropping segment", "path", path, "err", err)
			s.backoff = s.minBackoff
			return nil
		}

		slog.Warn("ship failed, retrying", "path", path, "err", err, "backoff", s.backoff)
		if sleepErr := s.sleep(ctx, s.backoff); sleepErr != nil {
			return sleepErr
		}
		s.backoff *= 2
		if s.backoff > s.maxBackoff {
			s.backoff = s.maxBackoff
		}
	}
}

// ship sends one segment. retry reports whether the failure (if any) is
// worth retrying: true for network errors and 5xx, false for 4xx (where
// the segment is dropped regardless).
func (s *Shipper) ship(ctx context.Context, path string) (retry bool, err error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path comes from the spool's own segment listing, not external input
	if err != nil {
		return false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, strings.NewReader(string(raw)))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("X-Batch-Id", batchID(path))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return true, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode / 100 {
	case 2:
		return false, os.Remove(path)
	case 4:
		if rmErr := os.Remove(path); rmErr != nil {
			return false, rmErr
		}
		return false, fmt.Errorf("ship %s: rejected with %s", path, resp.Status)
	default:
		return true, fmt.Errorf("ship %s: %s", path, resp.Status)
	}
}

// batchID derives the X-Batch-Id header value from a segment's filename.
func batchID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl.zst")
}
