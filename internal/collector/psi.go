// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/prometheus/procfs"

	"github.com/memaudit/memaudit/internal/model"
)

// PSI collects the psi record from /proc/pressure/memory.
type PSI struct {
	procRoot string
}

// NewPSI returns a PSI collector rooted at procRoot (normally "/proc";
// tests point it at a fixture directory).
func NewPSI(procRoot string) *PSI {
	return &PSI{procRoot: procRoot}
}

// Collect reads and parses /proc/pressure/memory. PSI is absent on some
// hardened kernels (CONFIG_PSI off): that's reported as (nil, nil), not an
// error, so callers can skip emitting the record instead of failing.
func (p *PSI) Collect() (*model.PSI, error) {
	if _, err := os.Stat(filepath.Join(p.procRoot, "pressure", "memory")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil //nolint:nilnil // absence is a valid, expected state here
		}
		return nil, fmt.Errorf("stat pressure/memory: %w", err)
	}

	fs, err := procfs.NewFS(p.procRoot)
	if err != nil {
		return nil, fmt.Errorf("open proc root %s: %w", p.procRoot, err)
	}
	stats, err := fs.PSIStatsForResource("memory")
	if err != nil {
		return nil, fmt.Errorf("read pressure/memory: %w", err)
	}

	return &model.PSI{
		SomeAvg10:  stats.Some.Avg10,
		SomeAvg60:  stats.Some.Avg60,
		SomeAvg300: stats.Some.Avg300,
		SomeTotal:  stats.Some.Total,
		FullAvg10:  stats.Full.Avg10,
		FullAvg60:  stats.Full.Avg60,
		FullAvg300: stats.Full.Avg300,
		FullTotal:  stats.Full.Total,
	}, nil
}
