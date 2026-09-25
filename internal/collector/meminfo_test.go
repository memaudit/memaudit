// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestMeminfoCollectGolden(t *testing.T) {
	got, err := NewMeminfo("../../testdata/container-linux-6.12/proc").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/container-linux-6.12/expected/host_mem.json", got)
}

// FuzzParseKReclaimableBytes fuzzes the one part of /proc/meminfo
// parsing this package still does by hand (everything else goes
// through prometheus/procfs, which is fuzzed upstream). Seeded from
// every real /proc/meminfo fixture in testdata/, across kernels with
// and without a KReclaimable line.
func FuzzParseKReclaimableBytes(f *testing.F) {
	fixtures := []string{
		"../../testdata/container-linux-6.12/proc/meminfo",
		"../../testdata/selftest/psi-absent/proc/meminfo",
		"../../testdata/selftest/full-caps/proc/meminfo",
		"../../testdata/scw-em-b111x/proc/meminfo",
		"../../testdata/edge-cases/psi-absent/proc/meminfo",
	}
	for _, path := range fixtures {
		b, err := os.ReadFile(path) //nolint:gosec // G304: fixed test-fixture paths under testdata/, not user input
		if err != nil {
			f.Fatalf("read fixture %s: %v", path, err)
		}
		f.Add(b)
	}
	f.Add([]byte(""))
	f.Add([]byte("KReclaimable:"))
	f.Add([]byte("KReclaimable: notanumber kB"))
	f.Add([]byte("KReclaimable: 99999999999999999999999999 kB"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic. Any result is either a real byte count or a
		// non-nil error - never both a nil error and a value that could
		// silently misrepresent what pre-4.20 kernels (correctly) report
		// as zero.
		kb, err := parseKReclaimableBytes(bytes.NewReader(data))
		if err == nil && kb == 0 && bytes.Contains(data, []byte("KReclaimable:")) {
			// A present-but-unparsed KReclaimable line must surface as
			// an error, never silently coerce to the same zero value
			// used for "kernel doesn't have this field at all". Compare
			// numerically, not by string equality - "000" is a
			// legitimately zero value, not an unparsed one.
			//
			// Only the FIRST matching line counts: parseKReclaimableBytes
			// scans and returns on its first match, same as any real
			// /proc/meminfo (which has at most one KReclaimable line) -
			// checking every line here would flag a later, unreached
			// line's value as a discrepancy that was never actually
			// reachable by the parser.
			for line := range strings.Lines(string(data)) {
				fields := strings.Fields(line)
				if len(fields) < 2 || fields[0] != "KReclaimable:" {
					continue
				}
				if n, err := strconv.ParseUint(fields[1], 10, 64); err == nil && n != 0 {
					t.Fatalf("KReclaimable line present with non-zero value %q but parse returned 0, nil: input %q", fields[1], data)
				}
				break
			}
		}
	})
}
