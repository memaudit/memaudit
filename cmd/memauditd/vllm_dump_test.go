// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestFormatMetricsSortedNoLabels(t *testing.T) {
	families := map[string]*dto.MetricFamily{
		"vllm:num_requests_running": {
			Name: new("vllm:num_requests_running"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: new(3.0)}},
			},
		},
		"vllm:gpu_cache_usage_perc": {
			Name: new("vllm:gpu_cache_usage_perc"),
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: new(0.42)}},
			},
		},
	}

	var buf bytes.Buffer
	formatMetrics(&buf, families)

	want := "vllm:gpu_cache_usage_perc 0.42\n" +
		"vllm:num_requests_running 3\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestFormatMetricsWithLabels(t *testing.T) {
	families := map[string]*dto.MetricFamily{
		"vllm:some_metric": {
			Name: new("vllm:some_metric"),
			Metric: []*dto.Metric{
				{
					Label:   []*dto.LabelPair{{Name: new("model"), Value: new("llama")}},
					Counter: &dto.Counter{Value: new(7.0)},
				},
			},
		},
	}

	var buf bytes.Buffer
	formatMetrics(&buf, families)

	want := `vllm:some_metric{model="llama"} 7` + "\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}
