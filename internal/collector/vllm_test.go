// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memaudit/memaudit/internal/model"
)

func defaultMetricMap() map[string]string {
	return map[string]string{
		"cache_usage":    "vllm:gpu_cache_usage_perc",
		"prefix_hits":    "vllm:gpu_prefix_cache_hits_total",
		"prefix_queries": "vllm:gpu_prefix_cache_queries_total",
		"preemptions":    "vllm:num_preemptions_total",
		"running":        "vllm:num_requests_running",
		"waiting":        "vllm:num_requests_waiting",
		"prompt_tokens":  "vllm:prompt_tokens_total",
		"gen_tokens":     "vllm:generation_tokens_total",
	}
}

const sampleVLLMMetrics = `# HELP vllm:gpu_cache_usage_perc GPU KV-cache usage percentage.
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc 0.42
# HELP vllm:num_requests_running Number of requests running.
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running 3
# HELP vllm:num_requests_waiting Number of requests waiting.
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting 1
# HELP vllm:prompt_tokens_total Prompt tokens processed.
# TYPE vllm:prompt_tokens_total counter
vllm:prompt_tokens_total 123456
# HELP vllm:generation_tokens_total Generation tokens produced.
# TYPE vllm:generation_tokens_total counter
vllm:generation_tokens_total 654321
# HELP vllm:num_preemptions_total Number of preemptions.
# TYPE vllm:num_preemptions_total counter
vllm:num_preemptions_total 7
# HELP vllm:gpu_prefix_cache_hits_total Prefix cache hits.
# TYPE vllm:gpu_prefix_cache_hits_total counter
vllm:gpu_prefix_cache_hits_total 999
# HELP vllm:gpu_prefix_cache_queries_total Prefix cache queries.
# TYPE vllm:gpu_prefix_cache_queries_total counter
vllm:gpu_prefix_cache_queries_total 1000
`

func TestVLLMCollectGolden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, sampleVLLMMetrics)
	}))
	defer srv.Close()

	v := NewVLLM([]string{srv.URL}, defaultMetricMap(), srv.Client())
	got, err := v.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	want := []model.VLLM{{
		Endpoint:      srv.URL,
		CacheUsage:    0.42,
		PrefixHits:    999,
		PrefixQueries: 1000,
		Preemptions:   7,
		Running:       3,
		Waiting:       1,
		PromptTokens:  123456,
		GenTokens:     654321,
	}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestVLLMCollectMultipleEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, sampleVLLMMetrics)
	}))
	defer srv.Close()

	v := NewVLLM([]string{srv.URL, srv.URL}, defaultMetricMap(), srv.Client())
	got, err := v.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
}

func TestVLLMCollectUnreachableEndpointSkippedNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, sampleVLLMMetrics)
	}))
	defer srv.Close()

	v := NewVLLM([]string{"http://127.0.0.1:1", srv.URL}, defaultMetricMap(), srv.Client())
	got, err := v.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1 (the reachable endpoint only)", len(got))
	}
	if got[0].Endpoint != srv.URL {
		t.Fatalf("got endpoint %q, want %q", got[0].Endpoint, srv.URL)
	}
}

func TestVLLMCollectUnmatchedMappingLeavesFieldZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "# TYPE vllm:gpu_cache_usage_perc gauge\nvllm:gpu_cache_usage_perc 0.1\n")
	}))
	defer srv.Close()

	metricMap := defaultMetricMap()
	metricMap["running"] = "vllm:this_metric_does_not_exist"

	v := NewVLLM([]string{srv.URL}, metricMap, srv.Client())
	got, err := v.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	if got[0].Running != 0 {
		t.Fatalf("Running = %v, want 0 (unmatched mapping)", got[0].Running)
	}
	if got[0].CacheUsage != 0.1 {
		t.Fatalf("CacheUsage = %v, want 0.1 (matched mapping unaffected)", got[0].CacheUsage)
	}
}
