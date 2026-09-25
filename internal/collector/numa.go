// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/memaudit/memaudit/pkg/model"
)

// Numa collects one numa_mem record per NUMA node from
// /sys/devices/system/node/nodeN/{meminfo,numastat}.
type Numa struct {
	sysRoot string
}

// NewNuma returns a Numa collector rooted at sysRoot (normally "/sys";
// tests point it at a fixture directory).
func NewNuma(sysRoot string) *Numa {
	return &Numa{sysRoot: sysRoot}
}

// Collect returns one record per node, sorted by node number. Hosts
// without NUMA sysfs exposed (single-node VMs, some containers) return a
// nil slice, not an error.
func (c *Numa) Collect() ([]model.NumaMem, error) {
	nodeDir := filepath.Join(c.sysRoot, "devices", "system", "node")
	entries, err := os.ReadDir(nodeDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", nodeDir, err)
	}

	var nodes []model.NumaMem
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "node") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), "node"))
		if err != nil {
			continue // e.g. a "node" symlink elsewhere in sysfs, not a nodeN dir
		}
		rec, err := c.collectNode(filepath.Join(nodeDir, e.Name()), n)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, rec)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Node < nodes[j].Node })
	return nodes, nil
}

var numaMeminfoFields = map[string]func(*model.NumaMem, uint64){
	"MemTotal":  func(r *model.NumaMem, n uint64) { r.MemTotal = n * 1024 },
	"MemFree":   func(r *model.NumaMem, n uint64) { r.MemFree = n * 1024 },
	"FilePages": func(r *model.NumaMem, n uint64) { r.FilePages = n * 1024 },
	"AnonPages": func(r *model.NumaMem, n uint64) { r.AnonPages = n * 1024 },
}

var numaStatFields = map[string]func(*model.NumaMem, uint64){
	"numa_hit":     func(r *model.NumaMem, n uint64) { r.NumaHit = n },
	"numa_miss":    func(r *model.NumaMem, n uint64) { r.NumaMiss = n },
	"numa_foreign": func(r *model.NumaMem, n uint64) { r.NumaForeign = n },
}

func (c *Numa) collectNode(dir string, node int) (model.NumaMem, error) {
	rec := model.NumaMem{Node: node}

	apply := func(setters map[string]func(*model.NumaMem, uint64)) func(key, val string) error {
		return func(key, val string) error {
			setter, ok := setters[key]
			if !ok {
				return nil
			}
			n, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return fmt.Errorf("parse %s: %w", key, err)
			}
			setter(&rec, n)
			return nil
		}
	}

	if err := scanNodeMeminfo(filepath.Join(dir, "meminfo"), apply(numaMeminfoFields)); err != nil {
		return model.NumaMem{}, err
	}
	if err := scanKV(filepath.Join(dir, "numastat"), apply(numaStatFields)); err != nil {
		return model.NumaMem{}, err
	}
	return rec, nil
}

// nodeMeminfoKV extracts the key/value pair from one "Node N Key:
// value [unit]" line used by .../nodeN/meminfo, as already split into
// fields. Named (not an inline closure) so it can be exercised directly
// by a fuzz test alongside scanReader.
func nodeMeminfoKV(fields []string) (key, val string, ok bool) {
	if len(fields) < 4 {
		return "", "", false
	}
	return strings.TrimSuffix(fields[2], ":"), fields[3], true
}

// plainKV extracts the key/value pair from one "key value" line, as
// used by .../nodeN/numastat and (via this same function) cgroup.go's
// memory.stat. Named (not an inline closure) so it can be exercised
// directly by a fuzz test alongside scanReader.
func plainKV(fields []string) (key, val string, ok bool) {
	if len(fields) < 2 {
		return "", "", false
	}
	return fields[0], fields[1], true
}

// scanNodeMeminfo scans the "Node N Key:  value [unit]" layout used by
// .../nodeN/meminfo.
func scanNodeMeminfo(path string, fn func(key, val string) error) error {
	return scanFile(path, func(fields []string) error {
		key, val, ok := nodeMeminfoKV(fields)
		if !ok {
			return nil
		}
		return fn(key, val)
	})
}

// scanKV scans the plain "key value" layout used by .../nodeN/numastat
// and cgroup.go's memory.stat.
func scanKV(path string, fn func(key, val string) error) error {
	return scanFile(path, func(fields []string) error {
		key, val, ok := plainKV(fields)
		if !ok {
			return nil
		}
		return fn(key, val)
	})
}

func scanFile(path string, fn func(fields []string) error) error {
	f, err := os.Open(path) //nolint:gosec // G304: path is built from an operator-supplied sys root, not untrusted input
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if err := scanReader(f, fn); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}

// scanReader is scanFile's line-scanning core, split out so it can be
// fuzzed directly against arbitrary bytes without a filesystem round
// trip per input.
func scanReader(r io.Reader, fn func(fields []string) error) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if err := fn(strings.Fields(scanner.Text())); err != nil {
			return err
		}
	}
	return scanner.Err()
}
