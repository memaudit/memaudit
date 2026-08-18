// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"reflect"
	"testing"
)

func TestDefaultVLLMMetricMap(t *testing.T) {
	got := Default().Collectors.VLLM.MetricMap
	want := map[string]string{
		"cache_usage":    "vllm:gpu_cache_usage_perc",
		"prefix_hits":    "vllm:gpu_prefix_cache_hits_total",
		"prefix_queries": "vllm:gpu_prefix_cache_queries_total",
		"preemptions":    "vllm:num_preemptions_total",
		"running":        "vllm:num_requests_running",
		"waiting":        "vllm:num_requests_waiting",
		"prompt_tokens":  "vllm:prompt_tokens_total",
		"gen_tokens":     "vllm:generation_tokens_total",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDefaultVLLMEndpointsEmpty(t *testing.T) {
	// No endpoints configured out of the box: vLLM collection is
	// implicitly off until an operator adds one, same convention as
	// every other opt-in collector.
	if got := Default().Collectors.VLLM.Endpoints; len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}
