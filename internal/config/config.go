// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package config loads and defaults the agent's /etc/memaudit/config.yaml.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Site       string           `yaml:"site"`
	IntervalS  int              `yaml:"interval_s"`
	Mode       string           `yaml:"mode"` // sampling | zerotouch
	Collectors CollectorsConfig `yaml:"collectors"`
	K8s        K8sConfig        `yaml:"k8s"`
	Spool      SpoolConfig      `yaml:"spool"`
	Ship       ShipConfig       `yaml:"ship"`
	Log        LogConfig        `yaml:"log"`
}

type CollectorsConfig struct {
	Cgroup CgroupConfig `yaml:"cgroup"`
	Damon  DamonConfig  `yaml:"damon"`
	NVML   NVMLConfig   `yaml:"nvml"`
	VLLM   VLLMConfig   `yaml:"vllm"`
}

type CgroupConfig struct {
	Enabled bool     `yaml:"enabled"`
	Globs   []string `yaml:"globs"`
	Max     int      `yaml:"max"`
}

type DamonConfig struct {
	Enabled    bool   `yaml:"enabled"`
	SampleUS   uint64 `yaml:"sample_us"`
	AggrUS     uint64 `yaml:"aggr_us"`
	MaxRegions uint64 `yaml:"max_regions"`
}

type NVMLConfig struct {
	// "auto" (default), "true", or "false" — auto loads the library and
	// disables the collector on failure instead of erroring.
	Enabled string `yaml:"enabled"`
}

type VLLMConfig struct {
	Endpoints []string          `yaml:"endpoints"`
	MetricMap map[string]string `yaml:"metric_map"`
}

type K8sConfig struct {
	Enrich    bool     `yaml:"enrich"`
	Kubelet   string   `yaml:"kubelet"`
	TokenPath string   `yaml:"token_path"`
	LabelKeys []string `yaml:"label_keys"`
}

type SpoolConfig struct {
	Dir      string `yaml:"dir"`
	MaxBytes int64  `yaml:"max_bytes"`
}

type ShipConfig struct {
	Mode      string `yaml:"mode"` // push | bundle
	URL       string `yaml:"url"`
	TokenFile string `yaml:"token_file"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

// Default returns the config defaults documented in the config reference,
// before any file is applied on top of it.
func Default() Config {
	return Config{
		IntervalS: 15,
		Mode:      "sampling",
		Collectors: CollectorsConfig{
			Cgroup: CgroupConfig{
				Enabled: true,
				Globs:   []string{"system.slice/*.service", "kubepods.slice/**"},
				Max:     500,
			},
			Damon: DamonConfig{
				Enabled:    true,
				SampleUS:   5_000,
				AggrUS:     100_000,
				MaxRegions: 1_000,
			},
			NVML: NVMLConfig{Enabled: "auto"},
		},
		K8s: K8sConfig{
			Kubelet:   "https://127.0.0.1:10250",
			LabelKeys: []string{"app"},
		},
		Spool: SpoolConfig{
			Dir:      "/var/lib/memaudit/spool",
			MaxBytes: 2 * 1024 * 1024 * 1024,
		},
		Ship: ShipConfig{Mode: "push"},
		Log:  LogConfig{Level: "info"},
	}
}

// Load reads path and applies its values on top of Default().
func Load(path string) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is operator-supplied via --config, not untrusted input
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}
