// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/memaudit/memaudit/internal/collector"
	"github.com/memaudit/memaudit/internal/config"
	"github.com/memaudit/memaudit/internal/k8s"
	"github.com/memaudit/memaudit/internal/model"
)

// tier is how much slower than the agent's base interval a source ticks,
// matching the default record intervals in the wire format spec: fast
// (interval_s itself), medium (2x), slow (4x) — e.g. at the default
// interval_s=15 that's 15s/30s/60s.
type tier int

const (
	tierFast   tier = 1
	tierMedium tier = 2
	tierSlow   tier = 4
)

// source ties one record type's collector to its tick tier and a uniform
// collect function.
type source struct {
	typ     string
	tier    tier
	collect func() ([]model.Envelope, error)
}

// buildSources returns the sources this build knows how to collect. Only
// cgroup is conditional on cfg (the other collectors here have no
// per-collector enable flag in the config schema — see Global
// Constraints). DAMON, NVML, and vLLM sources land in later builds
// alongside those collectors.
func buildSources(cfg config.CollectorsConfig, procRoot, sysRoot, site, host string, k8sClient k8s.Client, now func() time.Time) []source {
	srcs := []source{
		{typ: "host_mem", tier: tierFast, collect: single(site, host, "host_mem", now, collector.NewMeminfo(procRoot).Collect)},
		{typ: "vmstat", tier: tierFast, collect: single(site, host, "vmstat", now, collector.NewVmstat(procRoot).Collect)},
		{typ: "psi", tier: tierFast, collect: single(site, host, "psi", now, collector.NewPSI(procRoot).Collect)},
		{typ: "numa_mem", tier: tierMedium, collect: multi(site, host, "numa_mem", now, collector.NewNuma(sysRoot).Collect)},
		{typ: "hugepages", tier: tierSlow, collect: multi(site, host, "hugepages", now, collector.NewHugepages(sysRoot).Collect)},
	}

	if cfg.Cgroup.Enabled {
		cg := collector.NewCgroup(sysRoot, cfg.Cgroup.Globs, cfg.Cgroup.Max, k8sClient)
		srcs = append(srcs, source{
			typ:  "cgroup_mem",
			tier: tierMedium,
			collect: func() ([]model.Envelope, error) {
				recs, err := cg.Collect(context.Background())
				if err != nil {
					return nil, err
				}
				return envelopes(site, host, "cgroup_mem", now, recs)
			},
		})
	}

	return srcs
}

// single adapts a collector whose Collect returns (*T, error): a nil
// result (e.g. PSI absent) means "nothing to emit this tick," not an
// error, matching every existing collector's absence convention.
func single[T any](site, host, typ string, now func() time.Time, collect func() (*T, error)) func() ([]model.Envelope, error) {
	return func() ([]model.Envelope, error) {
		rec, err := collect()
		if err != nil {
			return nil, err
		}
		if rec == nil {
			return nil, nil
		}
		return envelopes(site, host, typ, now, []T{*rec})
	}
}

// multi adapts a collector whose Collect returns ([]T, error).
func multi[T any](site, host, typ string, now func() time.Time, collect func() ([]T, error)) func() ([]model.Envelope, error) {
	return func() ([]model.Envelope, error) {
		recs, err := collect()
		if err != nil {
			return nil, err
		}
		return envelopes(site, host, typ, now, recs)
	}
}

// envelopes marshals each of recs as typ's payload. Schema is always 1:
// every v1 record type is on its first payload version.
func envelopes[T any](site, host, typ string, now func() time.Time, recs []T) ([]model.Envelope, error) {
	ts := now().UTC()
	out := make([]model.Envelope, 0, len(recs))
	for _, rec := range recs {
		b, err := json.Marshal(rec)
		if err != nil {
			return nil, fmt.Errorf("marshal %s payload: %w", typ, err)
		}
		out = append(out, model.Envelope{
			TS:      ts,
			Site:    site,
			Host:    host,
			Type:    typ,
			Schema:  1,
			Payload: b,
		})
	}
	return out, nil
}
