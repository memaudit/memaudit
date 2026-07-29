// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const canned = `{
  "items": [
    {
      "metadata": {
        "uid": "1234abcd-5678-90ef-ab12-cdef34567890",
        "namespace": "default",
        "name": "web-0",
        "labels": {"app": "web", "team": "payments", "pod-template-hash": "abc123"}
      }
    }
  ]
}`

func newTestClient(t *testing.T, handler http.HandlerFunc, labelKeys []string) (*KubeletClient, *int) {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	c, err := NewKubeletClient(KubeletClientConfig{
		BaseURL:    srv.URL,
		Token:      "test-token",
		LabelKeys:  labelKeys,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewKubeletClient: %v", err)
	}
	return c, &requests
}

func TestLookupPodResolvesAndFiltersLabels(t *testing.T) {
	var gotAuth, gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(canned))
	}, []string{"app", "team"})

	meta, ok, err := c.LookupPod(context.Background(), "1234abcd-5678-90ef-ab12-cdef34567890")
	if err != nil {
		t.Fatalf("LookupPod: %v", err)
	}
	if !ok {
		t.Fatal("LookupPod: want found")
	}
	if meta.Namespace != "default" || meta.Name != "web-0" {
		t.Errorf("meta = %+v, want namespace=default name=web-0", meta)
	}
	want := map[string]string{"app": "web", "team": "payments"}
	if len(meta.Labels) != len(want) || meta.Labels["app"] != want["app"] || meta.Labels["team"] != want["team"] {
		t.Errorf("Labels = %v, want %v (pod-template-hash must be dropped, it's not allowlisted)", meta.Labels, want)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", gotAuth)
	}
	if gotPath != "/pods" {
		t.Errorf("path = %q, want /pods", gotPath)
	}
}

func TestLookupPodUnknownUIDNotFound(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(canned))
	}, []string{"app"})

	_, ok, err := c.LookupPod(context.Background(), "no-such-uid")
	if err != nil {
		t.Fatalf("LookupPod: %v", err)
	}
	if ok {
		t.Fatal("LookupPod: want not found")
	}
}

func TestLookupPodCachesWithinTTL(t *testing.T) {
	c, requests := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(canned))
	}, []string{"app"})

	fakeNow := time.Now()
	c.now = func() time.Time { return fakeNow }

	for range 3 {
		if _, _, err := c.LookupPod(context.Background(), "1234abcd-5678-90ef-ab12-cdef34567890"); err != nil {
			t.Fatalf("LookupPod: %v", err)
		}
	}
	if *requests != 1 {
		t.Errorf("requests = %d, want 1 (cache should absorb repeat lookups within the TTL)", *requests)
	}

	fakeNow = fakeNow.Add(61 * time.Second)
	if _, _, err := c.LookupPod(context.Background(), "1234abcd-5678-90ef-ab12-cdef34567890"); err != nil {
		t.Fatalf("LookupPod: %v", err)
	}
	if *requests != 2 {
		t.Errorf("requests = %d, want 2 (cache should refetch once the TTL elapses)", *requests)
	}
}

func TestLookupPodPropagatesKubeletError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}, nil)

	if _, _, err := c.LookupPod(context.Background(), "any"); err == nil {
		t.Fatal("LookupPod: want error on non-200 response")
	}
}
