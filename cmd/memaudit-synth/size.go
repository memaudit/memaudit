// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

var sizeSuffixes = []struct {
	suffix string
	mult   uint64
}{
	// Longest suffix first so e.g. "GiB" isn't matched as a bare "B".
	{"TiB", 1024 * 1024 * 1024 * 1024},
	{"GiB", 1024 * 1024 * 1024},
	{"MiB", 1024 * 1024},
	{"KiB", 1024},
}

// parseSize parses a human size string like "8GiB" or "512MiB" into bytes.
// Only binary (KiB/MiB/GiB/TiB) suffixes are accepted, matching how this
// tool's flags are documented; there is no bare-byte or decimal-suffix
// form.
func parseSize(s string) (uint64, error) {
	for _, sfx := range sizeSuffixes {
		if numPart, ok := strings.CutSuffix(s, sfx.suffix); ok {
			if numPart == "" {
				return 0, fmt.Errorf("parse size %q: missing number before %s", s, sfx.suffix)
			}
			n, err := strconv.ParseUint(numPart, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse size %q: %w", s, err)
			}
			if n > math.MaxUint64/sfx.mult {
				return 0, fmt.Errorf("parse size %q: overflows a 64-bit byte count", s)
			}
			return n * sfx.mult, nil
		}
	}
	return 0, fmt.Errorf("parse size %q: expected a KiB/MiB/GiB/TiB suffix", s)
}
