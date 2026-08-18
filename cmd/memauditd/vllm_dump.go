// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/memaudit/memaudit/internal/collector"
)

const vllmDumpTimeout = 10 * time.Second

// vllmDumpCmd prints every metric name/value a vLLM endpoint exposes —
// building collectors.vllm.metric_map for a new vLLM version is then a
// 2-minute operator task (compare this output to the metric names
// vllm.go's field table expects), not a release.
func vllmDumpCmd(args []string) {
	fs := flag.NewFlagSet("vllm-dump", flag.ExitOnError)
	endpoint := fs.String("endpoint", "", "vLLM base URL, e.g. http://127.0.0.1:8000")
	_ = fs.Parse(args) // ExitOnError: Parse never returns a non-nil error here

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "memauditd vllm-dump: --endpoint is required")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), vllmDumpTimeout)
	defer cancel()

	families, err := collector.ScrapeMetrics(ctx, &http.Client{Timeout: vllmDumpTimeout}, *endpoint)
	if err != nil {
		slog.Error("vllm-dump: scrape failed", "endpoint", *endpoint, "err", err)
		os.Exit(1)
	}

	formatMetrics(os.Stdout, families)
}

// formatMetrics writes one line per metric sample, sorted by family name
// for deterministic output: "<name>{label=\"value\",...} <value>", the
// label portion omitted when there are none.
func formatMetrics(w io.Writer, families map[string]*dto.MetricFamily) {
	names := make([]string, 0, len(families))
	for name := range families {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		for _, m := range families[name].GetMetric() {
			_, _ = fmt.Fprintf(w, "%s%s %s\n", name, formatLabels(m.GetLabel()), formatValue(m))
		}
	}
}

func formatLabels(labels []*dto.LabelPair) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, len(labels))
	for i, l := range labels {
		parts[i] = fmt.Sprintf("%s=%q", l.GetName(), l.GetValue())
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// formatValue handles the metric types vLLM actually exposes
// (Gauge/Counter/Untyped) directly; Histogram/Summary are noted rather
// than fully expanded (bucket/quantile dumping is more than this
// diagnostic tool needs).
func formatValue(m *dto.Metric) string {
	switch {
	case m.GetGauge() != nil:
		return formatFloat(m.GetGauge().GetValue())
	case m.GetCounter() != nil:
		return formatFloat(m.GetCounter().GetValue())
	case m.GetUntyped() != nil:
		return formatFloat(m.GetUntyped().GetValue())
	case m.GetHistogram() != nil:
		return fmt.Sprintf("(histogram, sample_count=%d sample_sum=%s)", m.GetHistogram().GetSampleCount(), formatFloat(m.GetHistogram().GetSampleSum()))
	case m.GetSummary() != nil:
		return fmt.Sprintf("(summary, sample_count=%d sample_sum=%s)", m.GetSummary().GetSampleCount(), formatFloat(m.GetSummary().GetSampleSum()))
	default:
		return "(no value)"
	}
}

func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", f), "0"), ".")
}
