// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package ship

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSegment(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatalf("write segment: %v", err)
	}
	return path
}

func TestRunShipsSegmentAndDeletesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	segPath := writeSegment(t, dir, "01BATCH.jsonl.zst", "fake-compressed-content")

	var gotAuth, gotBatchID, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotBatchID = r.Header.Get("X-Batch-Id")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sh := New(Config{URL: srv.URL, Token: "test-token"})

	listed := false
	list := func() ([]string, error) {
		if listed {
			return nil, nil
		}
		listed = true
		return []string{segPath}, nil
	}

	if err := sh.Run(context.Background(), list); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if gotBatchID != "01BATCH" {
		t.Errorf("X-Batch-Id = %q, want %q", gotBatchID, "01BATCH")
	}
	if gotBody != "fake-compressed-content" {
		t.Errorf("body = %q, want %q", gotBody, "fake-compressed-content")
	}

	if _, err := os.Stat(segPath); !os.IsNotExist(err) {
		t.Errorf("expected segment to be deleted after successful ship, stat err = %v", err)
	}
}

func TestRunDropsSegmentOn4xxWithoutRetry(t *testing.T) {
	dir := t.TempDir()
	segPath := writeSegment(t, dir, "01BAD.jsonl.zst", "schema too old")

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	sh := New(Config{URL: srv.URL, Token: "test-token"})

	listed := false
	list := func() ([]string, error) {
		if listed {
			return nil, nil
		}
		listed = true
		return []string{segPath}, nil
	}

	if err := sh.Run(context.Background(), list); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if requests != 1 {
		t.Errorf("expected exactly 1 request (no retry on 4xx), got %d", requests)
	}
	if _, err := os.Stat(segPath); !os.IsNotExist(err) {
		t.Errorf("expected segment to be dropped after a 4xx, stat err = %v", err)
	}
}

func TestRunRetriesRetryableFailureWithBackoff(t *testing.T) {
	dir := t.TempDir()
	segPath := writeSegment(t, dir, "01RETRY.jsonl.zst", "content")

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var slept []time.Duration
	fakeSleep := func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	sh := New(Config{
		URL:        srv.URL,
		Token:      "test-token",
		Sleep:      fakeSleep,
		MinBackoff: 10 * time.Millisecond,
		MaxBackoff: 1 * time.Second,
	})

	listed := false
	list := func() ([]string, error) {
		if listed {
			return nil, nil
		}
		listed = true
		return []string{segPath}, nil
	}

	if err := sh.Run(context.Background(), list); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if requests != 3 {
		t.Errorf("expected 3 requests (2 failures + 1 success), got %d", requests)
	}

	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	if len(slept) != len(want) {
		t.Fatalf("expected %d backoff sleeps, got %d: %v", len(want), len(slept), slept)
	}
	for i, w := range want {
		if slept[i] != w {
			t.Errorf("sleep[%d] = %v, want %v", i, slept[i], w)
		}
	}

	if _, err := os.Stat(segPath); !os.IsNotExist(err) {
		t.Errorf("expected segment to be deleted after eventual success, stat err = %v", err)
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	dir := t.TempDir()
	segPath := writeSegment(t, dir, "01STUCK.jsonl.zst", "content")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // always retryable
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	fakeSleep := func(ctx context.Context, d time.Duration) error {
		cancel() // simulate shutdown happening while the shipper is backing off
		return ctx.Err()
	}

	sh := New(Config{URL: srv.URL, Token: "test-token", Sleep: fakeSleep})

	list := func() ([]string, error) {
		return []string{segPath}, nil
	}

	err := sh.Run(ctx, list)
	if err == nil {
		t.Fatal("expected Run to return an error on context cancellation, got nil")
	}

	// The segment should still be on disk: it was never acknowledged.
	if _, statErr := os.Stat(segPath); statErr != nil {
		t.Errorf("expected segment to remain on disk after cancellation, stat err = %v", statErr)
	}
}
