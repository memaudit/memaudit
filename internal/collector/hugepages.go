// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/memaudit/memaudit/internal/model"
)

// Hugepages collects one record per (page size, NUMA node) from
// /sys/kernel/mm/hugepages and, when the host exposes per-node hugepages
// sysfs, /sys/devices/system/node/nodeN/hugepages.
type Hugepages struct {
	sysRoot string
}

// NewHugepages returns a Hugepages collector rooted at sysRoot (normally
// "/sys"; tests point it at a fixture directory).
func NewHugepages(sysRoot string) *Hugepages {
	return &Hugepages{sysRoot: sysRoot}
}

var hugepagesFields = map[string]func(*model.Hugepages, uint64){
	"nr_hugepages":      func(r *model.Hugepages, n uint64) { r.Total = n },
	"free_hugepages":    func(r *model.Hugepages, n uint64) { r.Free = n },
	"resv_hugepages":    func(r *model.Hugepages, n uint64) { r.Rsvd = n },
	"surplus_hugepages": func(r *model.Hugepages, n uint64) { r.Surp = n },
}

// Collect returns one record per configured page size, broken out by
// NUMA node when the host exposes per-node hugepages sysfs, or a single
// host-level record per size (Node == model.HugepagesNoNUMA) otherwise.
// Hosts with no hugepages sysfs at all (CONFIG_HUGETLB off, or a
// container without /sys/kernel/mm exposed) return a nil slice, not an
// error.
func (c *Hugepages) Collect() ([]model.Hugepages, error) {
	globalRoot := filepath.Join(c.sysRoot, "kernel", "mm", "hugepages")
	sizes, err := listHugepageSizes(globalRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	nodeRoot := filepath.Join(c.sysRoot, "devices", "system", "node")
	nodes, err := listHugepageNodes(nodeRoot)
	if err != nil {
		return nil, err
	}

	var out []model.Hugepages
	for _, sizeKB := range sizes {
		dirName := hugepagesDirName(sizeKB)
		recorded := false
		for _, node := range nodes {
			nodeDir := filepath.Join(nodeRoot, node.name, "hugepages", dirName)
			if _, statErr := os.Stat(nodeDir); os.IsNotExist(statErr) {
				continue
			}
			rec, readErr := readHugepagesDir(nodeDir, sizeKB, node.num)
			if readErr != nil {
				return nil, readErr
			}
			out = append(out, rec)
			recorded = true
		}
		if recorded {
			continue
		}
		rec, readErr := readHugepagesDir(filepath.Join(globalRoot, dirName), sizeKB, model.HugepagesNoNUMA)
		if readErr != nil {
			return nil, readErr
		}
		out = append(out, rec)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].SizeKB != out[j].SizeKB {
			return out[i].SizeKB < out[j].SizeKB
		}
		return out[i].Node < out[j].Node
	})
	return out, nil
}

type hugepageNode struct {
	name string
	num  int
}

// listHugepageNodes returns the NUMA nodes exposed under nodeRoot, or a
// nil slice (not an error) if the host has no NUMA sysfs at all.
func listHugepageNodes(nodeRoot string) ([]hugepageNode, error) {
	entries, err := os.ReadDir(nodeRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", nodeRoot, err)
	}

	var nodes []hugepageNode
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "node") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), "node"))
		if err != nil {
			continue // e.g. a "node" symlink elsewhere in sysfs, not a nodeN dir
		}
		nodes = append(nodes, hugepageNode{name: e.Name(), num: n})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].num < nodes[j].num })
	return nodes, nil
}

// listHugepageSizes returns the page sizes (KB) configured under root,
// parsed from "hugepages-<N>kB" directory names. The returned error, if
// any, is exactly os.ReadDir's — callers distinguish absence themselves.
func listHugepageSizes(root string) ([]uint64, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var sizes []uint64
	for _, e := range entries {
		sizeKB, ok := parseHugepagesDirName(e.Name())
		if !e.IsDir() || !ok {
			continue
		}
		sizes = append(sizes, sizeKB)
	}
	slices.Sort(sizes)
	return sizes, nil
}

func hugepagesDirName(sizeKB uint64) string {
	return fmt.Sprintf("hugepages-%dkB", sizeKB)
}

func parseHugepagesDirName(name string) (uint64, bool) {
	if !strings.HasPrefix(name, "hugepages-") || !strings.HasSuffix(name, "kB") {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(name, "hugepages-"), "kB"), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// readHugepagesDir reads one hugepages-<N>kB directory (global or
// per-node). Missing files (e.g. resv_hugepages, absent per-node on
// older kernels) are left zero, never an error.
func readHugepagesDir(dir string, sizeKB uint64, node int) (model.Hugepages, error) {
	rec := model.Hugepages{SizeKB: sizeKB, Node: node}
	for file, setter := range hugepagesFields {
		n, err := readUintFile(filepath.Join(dir, file))
		if err != nil {
			return model.Hugepages{}, err
		}
		setter(&rec, n)
	}
	return rec, nil
}

// readUintFile reads a single-line sysfs counter file. A missing file
// reads as zero, matching every other collector's "absence is fine"
// convention.
func readUintFile(path string) (uint64, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is built from an operator-supplied sys root, not untrusted input
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return n, nil
}
