// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Region is one DAMON-tracked memory region as read back from
// tried_regions. Start and End are DAMON's own native values: a
// half-open range [Start, End), unlike AddrRange (which mirrors
// /proc/iomem's inclusive-both-ends convention) — these are read directly
// from DAMON's sysfs output, not derived from /proc/iomem, so no
// conversion happens here.
type Region struct {
	Start, End      uint64
	NrAccesses, Age uint32
}

// Snapshot triggers a tried_regions update and reads back the regions
// DAMON currently tracks for s's scheme.
func (s *Session) Snapshot() ([]Region, error) {
	if err := s.writeFile(filepath.Join(s.kdamond, "state"), "update_schemes_tried_regions"); err != nil {
		return nil, fmt.Errorf("trigger tried_regions update: %w", err)
	}

	dir := filepath.Join(s.kdamond, "contexts", "0", "schemes", "0", "tried_regions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var indices []int
	for _, e := range entries {
		if !e.IsDir() {
			continue // e.g. total_bytes, a file alongside the numbered region dirs
		}
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		indices = append(indices, n)
	}
	sort.Ints(indices)

	regions := make([]Region, 0, len(indices))
	for _, i := range indices {
		rd := filepath.Join(dir, strconv.Itoa(i))
		start, err := readUint64File(filepath.Join(rd, "start"))
		if err != nil {
			return nil, err
		}
		end, err := readUint64File(filepath.Join(rd, "end"))
		if err != nil {
			return nil, err
		}
		nrAccesses, err := readUint32File(filepath.Join(rd, "nr_accesses"))
		if err != nil {
			return nil, err
		}
		age, err := readUint32File(filepath.Join(rd, "age"))
		if err != nil {
			return nil, err
		}
		regions = append(regions, Region{Start: start, End: end, NrAccesses: nrAccesses, Age: age})
	}
	return regions, nil
}

func readUint64File(path string) (uint64, error) {
	s, err := readSysfsFile(path)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return n, nil
}

func readUint32File(path string) (uint32, error) {
	s, err := readSysfsFile(path)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return uint32(n), nil
}
