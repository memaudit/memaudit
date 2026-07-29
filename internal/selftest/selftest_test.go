// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package selftest

import "testing"

func TestRunAllCapsOK(t *testing.T) {
	res := Run("../../testdata/selftest/full-caps/proc", "../../testdata/cgroup-v2-k8s/sys")

	for _, c := range res.Checks {
		if !c.OK {
			t.Errorf("check %q: got FAIL (%s), want OK", c.Name, c.Detail)
		}
	}
	if res.Verdict != "sampling mode available" {
		t.Errorf("Verdict = %q, want %q", res.Verdict, "sampling mode available")
	}
	if res.Failed() {
		t.Error("Failed() = true, want false")
	}
}

func TestRunPSIAbsent(t *testing.T) {
	res := Run("../../testdata/selftest/psi-absent/proc", "../../testdata/cgroup-v2-k8s/sys")

	psi := findCheck(t, res, "PSI")
	if psi.OK {
		t.Error("PSI check: got OK, want FAIL (fixture has no pressure/memory)")
	}
	if res.Verdict != "sampling mode available (PSI unavailable — stranded estimate not computed)" {
		t.Errorf("Verdict = %q", res.Verdict)
	}
	if res.Failed() {
		t.Error("Failed() = true, want false — PSI absence is a degraded mode, not a hard failure")
	}
}

func TestRunCgroupV1Host(t *testing.T) {
	res := Run("../../testdata/selftest/full-caps/proc", "../../testdata/edge-cases/cgroup-v1-host/sys")

	cgroup := findCheck(t, res, "cgroup v2 unified")
	if cgroup.OK {
		t.Error("cgroup v2 check: got OK, want FAIL (fixture is a v1 host)")
	}
	if res.Verdict != "sampling mode available (cgroup v1 host — host-level cgroup metrics only)" {
		t.Errorf("Verdict = %q", res.Verdict)
	}
	if res.Failed() {
		t.Error("Failed() = true, want false")
	}
}

func TestRunNoMeminfoFailsHard(t *testing.T) {
	res := Run(t.TempDir(), t.TempDir()) // empty dirs: no meminfo at all

	hostMem := findCheck(t, res, hostMemCheckName)
	if hostMem.OK {
		t.Error("host memory check: got OK, want FAIL")
	}
	if !res.Failed() {
		t.Error("Failed() = false, want true — no /proc/meminfo means no useful audit")
	}
}

func findCheck(t *testing.T, res Result, name string) Check {
	t.Helper()
	for _, c := range res.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %+v", name, res.Checks)
	return Check{}
}

func TestResultString(t *testing.T) {
	res := Result{
		Checks: []Check{
			{Name: "kernel 6.8.0-45-generic", OK: true},
			{Name: "PSI", OK: false, Detail: "PSI unavailable — stranded estimate not computed"},
		},
		Verdict: "sampling mode available (PSI unavailable — stranded estimate not computed)",
	}
	want := "kernel 6.8.0-45-generic ................ OK\n" +
		"PSI .................................... FAIL: PSI unavailable — stranded estimate not computed\n" +
		"verdict: sampling mode available (PSI unavailable — stranded estimate not computed)\n"
	if got := res.String(); got != want {
		t.Errorf("String() =\n%s\nwant:\n%s", got, want)
	}
}
