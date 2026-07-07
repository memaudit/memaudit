// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/memaudit/memaudit/internal/model"
)

// Vmstat collects the vmstat record from /proc/vmstat. There's no
// prometheus/procfs API for this file, so it's a small hand-rolled scanner
// that extracts only the keys memaudit cares about; unknown keys are
// ignored and keys missing on older kernels are left zero.
type Vmstat struct {
	procRoot string
}

// NewVmstat returns a Vmstat collector rooted at procRoot (normally
// "/proc"; tests point it at a fixture directory).
func NewVmstat(procRoot string) *Vmstat {
	return &Vmstat{procRoot: procRoot}
}

// vmstatFields maps the /proc/vmstat keys memaudit stores to a setter on
// the target struct.
var vmstatFields = map[string]func(*model.Vmstat, uint64){
	"pgscan_kswapd":            func(v *model.Vmstat, n uint64) { v.PgscanKswapd = n },
	"pgscan_direct":            func(v *model.Vmstat, n uint64) { v.PgscanDirect = n },
	"pgsteal_kswapd":           func(v *model.Vmstat, n uint64) { v.PgstealKswapd = n },
	"pgsteal_direct":           func(v *model.Vmstat, n uint64) { v.PgstealDirect = n },
	"pswpin":                   func(v *model.Vmstat, n uint64) { v.Pswpin = n },
	"pswpout":                  func(v *model.Vmstat, n uint64) { v.Pswpout = n },
	"workingset_refault_anon":  func(v *model.Vmstat, n uint64) { v.WorkingsetRefaultAnon = n },
	"workingset_refault_file":  func(v *model.Vmstat, n uint64) { v.WorkingsetRefaultFile = n },
	"workingset_activate_anon": func(v *model.Vmstat, n uint64) { v.WorkingsetActivateAnon = n },
	"workingset_activate_file": func(v *model.Vmstat, n uint64) { v.WorkingsetActivateFile = n },
	"oom_kill":                 func(v *model.Vmstat, n uint64) { v.OomKill = n },
	"pgmajfault":               func(v *model.Vmstat, n uint64) { v.PgmajFault = n },
}

// Collect reads and parses /proc/vmstat.
func (c *Vmstat) Collect() (*model.Vmstat, error) {
	path := filepath.Join(c.procRoot, "vmstat")
	f, err := os.Open(path) //nolint:gosec // G304: path is built from an operator-supplied proc root, not untrusted input
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	out := &model.Vmstat{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		setter, ok := vmstatFields[fields[0]]
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", fields[0], err)
		}
		setter(out, n)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return out, nil
}
