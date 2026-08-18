// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"path/filepath"
	"strconv"
)

// Config configures a DAMON monitoring session. Regions normally comes
// from ParseIomem.
type Config struct {
	Ops        string // "paddr"
	SampleUS   uint64
	AggrUS     uint64
	UpdateUS   uint64
	MinRegions uint64
	MaxRegions uint64
	Regions    []AddrRange
}

// Session is a running DAMON monitoring session, from Start to Stop.
type Session struct {
	kdamond   string
	writeFile sysfsWriteFunc
}

type sysfsWriteFunc func(path, value string) error

type sysfsWrite struct{ path, value string }

const maxU64Str = "18446744073709551615"

// Start configures and turns on a DAMON kdamond (index 0 under
// /sys/kernel/mm/damon/admin/kdamonds) monitoring cfg.Regions in paddr
// mode, with a single wide-open "stat" scheme (count accesses, act on
// nothing) — the scheme DAMON's sysfs interface requires you to declare
// even when you only want the tried_regions readout, not an actual
// action taken.
func Start(cfg Config) (*Session, error) {
	return start("/sys", cfg, writeSysfsFile)
}

func start(sysRoot string, cfg Config, writeFile sysfsWriteFunc) (*Session, error) {
	admin := filepath.Join(sysRoot, sysfsAdminRoot)
	k := filepath.Join(admin, "kdamonds", "0")
	c := filepath.Join(k, "contexts", "0")

	// Best-effort: a kdamond from an earlier, uncleanly-ended session may
	// still be on, and DAMON rejects config writes while a kdamond is
	// running. Ignore the error: on a fresh admin tree kdamonds/0 doesn't
	// exist yet, which is expected, not a problem.
	_ = writeFile(filepath.Join(k, "state"), "off")

	writes := []sysfsWrite{
		{filepath.Join(admin, "kdamonds", "nr_kdamonds"), "1"},
		{filepath.Join(k, "contexts", "nr_contexts"), "1"},
		{filepath.Join(c, "operations"), cfg.Ops},
		{filepath.Join(c, "monitoring_attrs", "intervals", "sample_us"), strconv.FormatUint(cfg.SampleUS, 10)},
		{filepath.Join(c, "monitoring_attrs", "intervals", "aggr_us"), strconv.FormatUint(cfg.AggrUS, 10)},
		{filepath.Join(c, "monitoring_attrs", "intervals", "update_us"), strconv.FormatUint(cfg.UpdateUS, 10)},
		{filepath.Join(c, "monitoring_attrs", "nr_regions", "min"), strconv.FormatUint(cfg.MinRegions, 10)},
		{filepath.Join(c, "monitoring_attrs", "nr_regions", "max"), strconv.FormatUint(cfg.MaxRegions, 10)},
		{filepath.Join(c, "targets", "nr_targets"), "1"},
		{filepath.Join(c, "targets", "0", "regions", "nr_regions"), strconv.Itoa(len(cfg.Regions))},
	}
	for i, r := range cfg.Regions {
		rd := filepath.Join(c, "targets", "0", "regions", strconv.Itoa(i))
		writes = append(writes,
			sysfsWrite{filepath.Join(rd, "start"), strconv.FormatUint(r.Start, 10)},
			// AddrRange.End is inclusive (matches /proc/iomem); DAMON's
			// sysfs region end is exclusive ([start, end)), hence +1.
			sysfsWrite{filepath.Join(rd, "end"), strconv.FormatUint(r.End+1, 10)},
		)
	}
	s := filepath.Join(c, "schemes", "0")
	writes = append(writes,
		sysfsWrite{filepath.Join(c, "schemes", "nr_schemes"), "1"},
		sysfsWrite{filepath.Join(s, "action"), "stat"},
		sysfsWrite{filepath.Join(s, "access_pattern", "sz", "min"), "0"},
		sysfsWrite{filepath.Join(s, "access_pattern", "sz", "max"), maxU64Str},
		sysfsWrite{filepath.Join(s, "access_pattern", "nr_accesses", "min"), "0"},
		sysfsWrite{filepath.Join(s, "access_pattern", "nr_accesses", "max"), maxU64Str},
		sysfsWrite{filepath.Join(s, "access_pattern", "age", "min"), "0"},
		sysfsWrite{filepath.Join(s, "access_pattern", "age", "max"), maxU64Str},
	)

	for _, w := range writes {
		if err := writeFile(w.path, w.value); err != nil {
			return nil, err
		}
	}

	if err := writeFile(filepath.Join(k, "state"), "on"); err != nil {
		return nil, err
	}

	return &Session{kdamond: k, writeFile: writeFile}, nil
}

// Stop turns off s's kdamond. It does not tear down the sysfs tree —
// a subsequent Start reconfigures and reuses the same kdamond index.
func (s *Session) Stop() error {
	return s.writeFile(filepath.Join(s.kdamond, "state"), "off")
}
