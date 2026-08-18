// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	promodel "github.com/prometheus/common/model"

	"github.com/memaudit/memaudit/internal/model"
)

const httpClientTimeout = 10 * time.Second

// vllmFields maps this project's internal field names to the model.VLLM
// setter for that field. The actual Prometheus metric name for each is
// operator-configured (collectors.vllm.metric_map) since vLLM renames
// metrics across versions.
var vllmFields = map[string]func(*model.VLLM, float64){
	"cache_usage":    func(r *model.VLLM, v float64) { r.CacheUsage = v },
	"prefix_hits":    func(r *model.VLLM, v float64) { r.PrefixHits = v },
	"prefix_queries": func(r *model.VLLM, v float64) { r.PrefixQueries = v },
	"preemptions":    func(r *model.VLLM, v float64) { r.Preemptions = v },
	"running":        func(r *model.VLLM, v float64) { r.Running = v },
	"waiting":        func(r *model.VLLM, v float64) { r.Waiting = v },
	"prompt_tokens":  func(r *model.VLLM, v float64) { r.PromptTokens = v },
	"gen_tokens":     func(r *model.VLLM, v float64) { r.GenTokens = v },
}

// VLLM scrapes each configured vLLM endpoint's Prometheus /metrics and
// maps the configured names onto a vllm record.
type VLLM struct {
	endpoints  []string
	metricMap  map[string]string
	httpClient *http.Client

	// warned tracks which unmatched metric names have already been
	// logged, so a persistently-missing mapping logs once, not every
	// tick.
	warned map[string]bool
}

// NewVLLM returns a VLLM collector. httpClient defaults to a plain
// 10s-timeout client if nil; tests override it (e.g. an httptest.Server's
// own client).
func NewVLLM(endpoints []string, metricMap map[string]string, httpClient *http.Client) *VLLM {
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	return &VLLM{
		endpoints:  endpoints,
		metricMap:  metricMap,
		httpClient: httpClient,
		warned:     map[string]bool{},
	}
}

// Collect scrapes every configured endpoint. An unreachable endpoint is
// logged and skipped — one down vLLM instance shouldn't lose records from
// the others.
func (v *VLLM) Collect(ctx context.Context) ([]model.VLLM, error) {
	var out []model.VLLM
	for _, endpoint := range v.endpoints {
		families, err := ScrapeMetrics(ctx, v.httpClient, endpoint)
		if err != nil {
			slog.Warn("vllm: scrape failed, skipping this endpoint", "endpoint", endpoint, "err", err)
			continue
		}
		out = append(out, v.buildRecord(endpoint, families))
	}
	return out, nil
}

func (v *VLLM) buildRecord(endpoint string, families map[string]*dto.MetricFamily) model.VLLM {
	rec := model.VLLM{Endpoint: endpoint}
	for internalName, setter := range vllmFields {
		metricName := v.metricMap[internalName]
		if metricName == "" {
			continue
		}
		mf, ok := families[metricName]
		if !ok || len(mf.GetMetric()) == 0 {
			v.warnUnmatchedOnce(metricName)
			continue
		}
		setter(&rec, metricValue(mf.GetMetric()[0]))
	}
	return rec
}

func (v *VLLM) warnUnmatchedOnce(metricName string) {
	if v.warned[metricName] {
		return
	}
	v.warned[metricName] = true
	slog.Warn("vllm: configured metric not found in scrape", "metric", metricName)
}

// ScrapeMetrics GETs endpoint's /metrics and parses it as Prometheus text
// exposition format. Exported so `memauditd vllm-dump` can reuse the same
// scrape/parse path the real collector uses.
func ScrapeMetrics(ctx context.Context, httpClient *http.Client, endpoint string) (map[string]*dto.MetricFamily, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s/metrics: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s/metrics: %s", endpoint, resp.Status)
	}

	// vLLM's metric names use the classic colon-including convention
	// (e.g. "vllm:gpu_cache_usage_perc") -- LegacyValidation matches
	// that. The zero-value TextParser panics in this library version
	// ("Invalid name validation scheme requested: unset"), confirmed by
	// running it, not assumed from docs.
	parser := expfmt.NewTextParser(promodel.LegacyValidation)
	families, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse %s/metrics: %w", endpoint, err)
	}
	return families, nil
}

// metricValue extracts a metric's value regardless of its Prometheus
// type (Gauge/Counter/Untyped) — all three Get*().GetValue() accessors
// are nil-safe.
func metricValue(m *dto.Metric) float64 {
	switch {
	case m.GetGauge() != nil:
		return m.GetGauge().GetValue()
	case m.GetCounter() != nil:
		return m.GetCounter().GetValue()
	case m.GetUntyped() != nil:
		return m.GetUntyped().GetValue()
	default:
		return 0
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: httpClientTimeout}
}
