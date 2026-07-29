// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package selftest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Check is one row of the capability matrix.
type Check struct {
	Name string // e.g. "kernel 6.8.0-45-generic", "cgroup v2 unified", "PSI"
	OK   bool
	// Detail explains a failed check; empty when OK.
	Detail string
}

// Result is the full capability matrix plus the one-line verdict.
type Result struct {
	Checks  []Check
	Verdict string
}

// hostMemCheckName identifies the one check whose failure means the host
// can't produce a useful audit at all: every collector and every
// degraded-mode fallback assumes /proc/meminfo is readable. Every other
// check failing just narrows which fallback applies — see Verdict.
const hostMemCheckName = "host memory (/proc/meminfo)"

// Run inspects procRoot (normally "/proc") and sysRoot (normally "/sys")
// and returns the capability matrix. Only checks for collectors this
// build actually ships are included — DAMON, NVML, and vLLM rows land in
// later builds alongside those collectors.
func Run(procRoot, sysRoot string) Result {
	checks := []Check{
		hostMemCheck(procRoot),
		kernelCheck(procRoot),
		cgroupV2Check(sysRoot),
		psiCheck(procRoot),
	}
	return Result{Checks: checks, Verdict: verdict(checks)}
}

// Failed reports whether the host can't produce a useful audit at all —
// the one condition that should make `memauditd selftest` exit non-zero.
func (r Result) Failed() bool {
	for _, c := range r.Checks {
		if c.Name == hostMemCheckName {
			return !c.OK
		}
	}
	return false
}

// String renders the aligned capability matrix followed by the verdict
// line, matching the format shown to prospects during a compatibility
// call.
func (r Result) String() string {
	const width = 40
	var b strings.Builder
	for _, c := range r.Checks {
		status := "OK"
		if !c.OK {
			status = "FAIL"
			if c.Detail != "" {
				status = "FAIL: " + c.Detail
			}
		}
		pad := width - len(c.Name) - 1
		if pad < 1 {
			pad = 1
		}
		fmt.Fprintf(&b, "%s %s %s\n", c.Name, strings.Repeat(".", pad), status)
	}
	fmt.Fprintf(&b, "verdict: %s\n", r.Verdict)
	return b.String()
}

func hostMemCheck(procRoot string) Check {
	if _, err := os.Stat(filepath.Join(procRoot, "meminfo")); err != nil {
		return Check{Name: hostMemCheckName, Detail: err.Error()}
	}
	return Check{Name: hostMemCheckName, OK: true}
}

func kernelCheck(procRoot string) Check {
	path := filepath.Join(procRoot, "sys", "kernel", "osrelease")
	b, err := os.ReadFile(path) //nolint:gosec // G304: procRoot is operator-supplied ("/proc" in production, a fixture dir in tests), not untrusted input
	if err != nil {
		return Check{Name: "kernel", Detail: err.Error()}
	}
	release := strings.TrimSpace(string(b))
	return Check{Name: "kernel " + release, OK: true}
}

func cgroupV2Check(sysRoot string) Check {
	path := filepath.Join(sysRoot, "fs", "cgroup", "cgroup.controllers")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Check{Name: "cgroup v2 unified", Detail: "cgroup v1 host — host-level cgroup metrics only"}
		}
		return Check{Name: "cgroup v2 unified", Detail: err.Error()}
	}
	return Check{Name: "cgroup v2 unified", OK: true}
}

func psiCheck(procRoot string) Check {
	path := filepath.Join(procRoot, "pressure", "memory")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Check{Name: "PSI", Detail: "PSI unavailable — stranded estimate not computed"}
		}
		return Check{Name: "PSI", Detail: err.Error()}
	}
	return Check{Name: "PSI", OK: true}
}

// verdict summarizes checks into the one-line verdict shown to a
// prospect. A failed host-memory check means no useful audit is
// possible at all; every other failure is folded in as a parenthetical
// note rather than blocking sampling mode.
func verdict(checks []Check) string {
	for _, c := range checks {
		if c.Name == hostMemCheckName && !c.OK {
			return "no useful audit possible: " + c.Detail
		}
	}
	var notes []string
	for _, c := range checks {
		if !c.OK && c.Name != hostMemCheckName {
			notes = append(notes, c.Detail)
		}
	}
	if len(notes) == 0 {
		return "sampling mode available"
	}
	return "sampling mode available (" + strings.Join(notes, "; ") + ")"
}
