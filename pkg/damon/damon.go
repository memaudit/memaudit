// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const sysfsAdminRoot = "kernel/mm/damon/admin"

// Caps describes what a running kernel's DAMON support allows, matching
// the three rungs `memauditd selftest` reports (sysfs, paddr,
// tried_regions):
//
//   - no sysfs interface at all (pre-5.18, or CONFIG_DAMON_SYSFS off): all
//     fields false.
//   - sysfs present, scheme-stats only (5.18-6.1): Sysfs and Paddr true,
//     TriedRegions false.
//   - sysfs present with tried_regions readout (>=6.2): all true, full
//     histogram mode available.
type Caps struct {
	// Sysfs reports whether /sys/kernel/mm/damon/admin exists.
	Sysfs bool
	// Paddr reports whether physical-address-space monitoring
	// (operations=paddr) is supported. This mirrors Sysfs: the only way
	// to verify it independently is to read a context's
	// avail_operations file, which does not exist until a context is
	// created — i.e. verifying it requires a sysfs write. Detect stays
	// read-only (this project's sampling-mode trust story is built on
	// selftest/detection never touching /sys), so this is a
	// best-effort assumption; Start is the authoritative check and
	// returns a clear error if paddr genuinely isn't supported.
	Paddr bool
	// TriedRegions reports whether the kernel is new enough (>=6.2) to
	// expose the tried_regions readout that full histogram mode needs.
	TriedRegions bool
}

// Detect reports the running kernel's DAMON capabilities. Absence at any
// rung is a valid, expected state (nil error), matching every other
// capability check in this project.
func Detect() (Caps, error) {
	return detect("/proc", "/sys")
}

func detect(procRoot, sysRoot string) (Caps, error) {
	nrKdamonds := filepath.Join(sysRoot, sysfsAdminRoot, "kdamonds", "nr_kdamonds")
	if _, err := os.Stat(nrKdamonds); err != nil {
		return Caps{}, nil //nolint:nilnil // absence is a valid, expected state here
	}

	release, err := readKernelRelease(procRoot)
	if err != nil {
		return Caps{}, fmt.Errorf("read kernel release: %w", err)
	}
	triedRegions := kernelAtLeast(release, 6, 2)

	return Caps{Sysfs: true, Paddr: true, TriedRegions: triedRegions}, nil
}

func readKernelRelease(procRoot string) (string, error) {
	path := filepath.Join(procRoot, "sys", "kernel", "osrelease")
	b, err := os.ReadFile(path) //nolint:gosec // G304: procRoot is operator-supplied ("/proc" in production, a fixture dir in tests), not untrusted input
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// kernelAtLeast reports whether release's major.minor version is >= the
// given major.minor, tolerating the usual distro suffixes (e.g.
// "6.8.0-45-generic", "7.1.6-201.fc44.x86_64").
func kernelAtLeast(release string, wantMajor, wantMinor int) bool {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	if major != wantMajor {
		return major > wantMajor
	}
	return minor >= wantMinor
}
