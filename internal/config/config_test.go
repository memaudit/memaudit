// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
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

func TestDefaultDebugPprofAddrEmpty(t *testing.T) {
	// Off by default: the debug endpoint exposes runtime internals and
	// must be an operator's deliberate choice, never active out of the
	// box.
	if got := Default().Debug.PprofAddr; got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestLoadParsesDebugPprofAddr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("debug:\n  pprof_addr: 127.0.0.1:6060\n"), 0o600); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Debug.PprofAddr; got != "127.0.0.1:6060" {
		t.Fatalf("got %q, want %q", got, "127.0.0.1:6060")
	}
}
