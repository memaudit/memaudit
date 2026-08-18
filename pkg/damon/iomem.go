// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// AddrRange is a physical address range as reported by /proc/iomem:
// inclusive of both Start and End (the last address in the range),
// matching /proc/iomem's own convention. Callers writing this to DAMON's
// sysfs region files must convert to DAMON's half-open [start, end)
// convention themselves (End+1) — this type deliberately stays a faithful
// parse of the source format rather than baking in that conversion.
type AddrRange struct {
	Start uint64
	End   uint64
}

// ErrIomemMasked is returned by ParseIomem when /proc/iomem's addresses
// are masked (every range reads back as 00000000-00000000, labels
// intact) — the kernel does this for unprivileged readers.
var ErrIomemMasked = errors.New("addresses masked: /proc/iomem requires root")

// ParseIomem returns the "System RAM" address ranges from /proc/iomem.
// Only top-level (unindented) entries are returned; indented lines are
// subdivisions of a System RAM range (kernel code/data/bss, etc.), not
// separate regions. Requires root — see ErrIomemMasked.
func ParseIomem() ([]AddrRange, error) {
	return parseIomemFile("/proc/iomem")
}

func parseIomemFile(path string) ([]AddrRange, error) {
	f, err := os.Open(path) //nolint:gosec // G304: fixed path, not caller-supplied
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return parseIomemReader(f)
}

func parseIomemReader(r io.Reader) ([]AddrRange, error) {
	var ranges []AddrRange
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue // indented: a subdivision of an enclosing range, not top-level
		}

		addrs, label, ok := strings.Cut(line, " : ")
		if !ok || label != "System RAM" {
			continue
		}

		startHex, endHex, ok := strings.Cut(addrs, "-")
		if !ok {
			continue
		}
		start, err := strconv.ParseUint(startHex, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("parse iomem start %q: %w", startHex, err)
		}
		end, err := strconv.ParseUint(endHex, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("parse iomem end %q: %w", endHex, err)
		}
		if start == 0 && end == 0 {
			return nil, ErrIomemMasked
		}
		ranges = append(ranges, AddrRange{Start: start, End: end})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read iomem: %w", err)
	}
	return ranges, nil
}
