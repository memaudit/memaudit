// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/memaudit/memaudit/internal/k8s"
	"github.com/memaudit/memaudit/pkg/model"
)

// maxCgroupDepth caps how many path segments below /sys/fs/cgroup are
// considered, matching the config's depth cap.
const maxCgroupDepth = 5

// Cgroup walks cgroup v2's unified hierarchy under /sys/fs/cgroup,
// collecting one record per selected cgroup.
type Cgroup struct {
	sysRoot   string
	globSegs  [][]string
	max       int
	k8sClient k8s.Client
}

// NewCgroup returns a Cgroup collector rooted at sysRoot (normally
// "/sys"; tests point it at a fixture directory), selecting cgroups
// whose path relative to /sys/fs/cgroup matches any of globs ("*"
// matches exactly one path segment, "**" matches zero or more), capped
// at max selected cgroups. k8sClient enables pod/namespace/label
// enrichment for cgroups under a kubepods.slice path; pass nil to
// disable enrichment entirely.
func NewCgroup(sysRoot string, globs []string, max int, k8sClient k8s.Client) *Cgroup {
	segs := make([][]string, len(globs))
	for i, g := range globs {
		segs[i] = strings.Split(g, "/")
	}
	return &Cgroup{sysRoot: sysRoot, globSegs: segs, max: max, k8sClient: k8sClient}
}

// Collect walks /sys/fs/cgroup and returns one record per selected
// cgroup, most-shallow first. Cgroup v1 hosts (no unified
// cgroup.controllers file at the root) return a nil slice, not an
// error — v1 hosts degrade to host-level metrics only. Selection beyond
// max cgroups is logged and silently truncated, never an error.
func (c *Cgroup) Collect(ctx context.Context) ([]model.CgroupMem, error) {
	root := filepath.Join(c.sysRoot, "fs", "cgroup")
	if _, err := os.Stat(filepath.Join(root, "cgroup.controllers")); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}

	var out []model.CgroupMem
	truncated := false
	if err := c.walk(ctx, root, nil, 0, &out, &truncated); err != nil {
		return nil, err
	}
	if truncated {
		slog.Warn("cgroup collector: selection truncated", "max", c.max)
	}
	return out, nil
}

// walk visits dir's child cgroups (segs identifies dir's own path
// relative to the cgroup root; depth is len(segs)). It only descends
// into a child when there's still a glob that child's path could grow
// into a match for, which keeps an agent with the default globs from
// walking irrelevant subtrees like a busy user.slice.
func (c *Cgroup) walk(ctx context.Context, dir string, segs []string, depth int, out *[]model.CgroupMem, truncated *bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	for _, e := range entries {
		if len(*out) >= c.max {
			*truncated = true
			return nil
		}
		if !e.IsDir() {
			continue
		}

		childSegs := make([]string, len(segs)+1)
		copy(childSegs, segs)
		childSegs[len(segs)] = e.Name()

		if matchesAny(childSegs, c.globSegs) {
			rec, err := c.readCgroup(ctx, filepath.Join(dir, e.Name()), strings.Join(childSegs, "/"))
			if err != nil {
				return err
			}
			*out = append(*out, rec)
		}

		if depth+1 < maxCgroupDepth && isPossiblePrefixForAny(childSegs, c.globSegs) {
			if err := c.walk(ctx, filepath.Join(dir, e.Name()), childSegs, depth+1, out, truncated); err != nil {
				return err
			}
		}
	}
	return nil
}

// matchesAny reports whether segs matches any of globs.
func matchesAny(segs []string, globs [][]string) bool {
	for _, g := range globs {
		if matchGlob(g, segs) {
			return true
		}
	}
	return false
}

// matchGlob matches segs against pattern segment-by-segment: "*" (or
// any filepath.Match wildcard within a segment, e.g. "*.service")
// consumes exactly one segment, "**" consumes zero or more.
func matchGlob(pattern, segs []string) bool {
	if len(pattern) == 0 {
		return len(segs) == 0
	}
	if pattern[0] == "**" {
		if matchGlob(pattern[1:], segs) {
			return true
		}
		if len(segs) == 0 {
			return false
		}
		return matchGlob(pattern, segs[1:])
	}
	if len(segs) == 0 {
		return false
	}
	if ok, err := filepath.Match(pattern[0], segs[0]); err != nil || !ok {
		return false
	}
	return matchGlob(pattern[1:], segs[1:])
}

// isPossiblePrefixForAny reports whether it's still worth descending
// below segs: whether segs could be extended into a match for at least
// one glob in globs.
func isPossiblePrefixForAny(segs []string, globs [][]string) bool {
	for _, g := range globs {
		if isPossiblePrefix(segs, g) {
			return true
		}
	}
	return false
}

func isPossiblePrefix(segs, pattern []string) bool {
	for i, seg := range segs {
		if i >= len(pattern) {
			return false
		}
		if pattern[i] == "**" {
			return true
		}
		if ok, err := filepath.Match(pattern[i], seg); err != nil || !ok {
			return false
		}
	}
	return true
}

var cgroupStatFields = map[string]func(*model.CgroupMem, uint64){
	"anon":                     func(r *model.CgroupMem, n uint64) { r.Anon = n },
	"file":                     func(r *model.CgroupMem, n uint64) { r.File = n },
	"kernel":                   func(r *model.CgroupMem, n uint64) { r.Kernel = n },
	"slab_reclaimable":         func(r *model.CgroupMem, n uint64) { r.SlabReclaimable = n },
	"slab_unreclaimable":       func(r *model.CgroupMem, n uint64) { r.SlabUnreclaimable = n },
	"file_mapped":              func(r *model.CgroupMem, n uint64) { r.FileMapped = n },
	"file_dirty":               func(r *model.CgroupMem, n uint64) { r.FileDirty = n },
	"inactive_anon":            func(r *model.CgroupMem, n uint64) { r.InactiveAnon = n },
	"active_anon":              func(r *model.CgroupMem, n uint64) { r.ActiveAnon = n },
	"inactive_file":            func(r *model.CgroupMem, n uint64) { r.InactiveFile = n },
	"active_file":              func(r *model.CgroupMem, n uint64) { r.ActiveFile = n },
	"workingset_refault_anon":  func(r *model.CgroupMem, n uint64) { r.WorkingsetRefaultAnon = n },
	"workingset_refault_file":  func(r *model.CgroupMem, n uint64) { r.WorkingsetRefaultFile = n },
	"workingset_activate_anon": func(r *model.CgroupMem, n uint64) { r.WorkingsetActivateAnon = n },
	"workingset_activate_file": func(r *model.CgroupMem, n uint64) { r.WorkingsetActivateFile = n },
	"pgscan":                   func(r *model.CgroupMem, n uint64) { r.Pgscan = n },
	"pgsteal":                  func(r *model.CgroupMem, n uint64) { r.Pgsteal = n },
}

// readCgroup reads every field memaudit stores for one selected cgroup
// directory and, when enrichment is on, attaches k8s metadata.
func (c *Cgroup) readCgroup(ctx context.Context, dir, relPath string) (model.CgroupMem, error) {
	rec := model.CgroupMem{Cgroup: relPath}

	var err error
	if rec.Current, err = readUintFile(filepath.Join(dir, "memory.current")); err != nil {
		return model.CgroupMem{}, err
	}
	if rec.SwapCurrent, err = readUintFile(filepath.Join(dir, "memory.swap.current")); err != nil {
		return model.CgroupMem{}, err
	}
	if rec.Max, err = readNullableUint(filepath.Join(dir, "memory.max")); err != nil {
		return model.CgroupMem{}, err
	}
	if rec.Min, err = readNullableUint(filepath.Join(dir, "memory.min")); err != nil {
		return model.CgroupMem{}, err
	}
	if rec.Low, err = readNullableUint(filepath.Join(dir, "memory.low")); err != nil {
		return model.CgroupMem{}, err
	}
	if rec.High, err = readNullableUint(filepath.Join(dir, "memory.high")); err != nil {
		return model.CgroupMem{}, err
	}
	if rec.Peak, err = readNullableUint(filepath.Join(dir, "memory.peak")); err != nil {
		return model.CgroupMem{}, err
	}
	if rec.PSI, err = readCgroupPSI(filepath.Join(dir, "memory.pressure")); err != nil {
		return model.CgroupMem{}, err
	}
	if err := c.readMemoryStat(&rec, dir); err != nil {
		return model.CgroupMem{}, err
	}

	c.enrich(ctx, &rec)
	return rec, nil
}

// readMemoryStat parses dir/memory.stat's "key value" lines into rec. A
// missing file (memory controller not enabled on this cgroup) leaves
// rec's stat fields zero, never an error.
func (c *Cgroup) readMemoryStat(rec *model.CgroupMem, dir string) error {
	err := scanKV(filepath.Join(dir, "memory.stat"), func(key, val string) error {
		setter, ok := cgroupStatFields[key]
		if !ok {
			return nil
		}
		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("parse %s: %w", key, err)
		}
		setter(rec, n)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// enrich attaches k8s pod/namespace/label metadata to rec when
// enrichment is enabled and rec's cgroup path resolves to a pod UID. A
// kubelet lookup failure is logged and otherwise ignored — enrichment is
// a bonus, not a reason to drop a memory sample.
func (c *Cgroup) enrich(ctx context.Context, rec *model.CgroupMem) {
	if c.k8sClient == nil {
		return
	}
	if cid, ok := k8s.ContainerID(rec.Cgroup); ok {
		rec.K8sContainerID = cid
	}
	uid, ok := k8s.PodUID(rec.Cgroup)
	if !ok {
		return
	}
	meta, found, err := c.k8sClient.LookupPod(ctx, uid)
	if err != nil {
		slog.Warn("cgroup collector: kubelet lookup failed", "uid", uid, "err", err)
		return
	}
	if !found {
		return
	}
	rec.K8sNamespace = meta.Namespace
	rec.K8sPod = meta.Name
	rec.K8sLabels = meta.Labels
}

// readNullableUint reads a cgroup memory.{max,min,low,high,peak}-style
// file: a missing file or the literal value "max" both mean "no limit
// set" (nil), never an error.
func readNullableUint(path string) (*uint64, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is built from an operator-supplied sys root, not untrusted input
	if os.IsNotExist(err) {
		return nil, nil //nolint:nilnil // absence means "no limit set", a valid state
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" || s == "max" {
		return nil, nil //nolint:nilnil // "max" means "no limit set", a valid state
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &n, nil
}

// readCgroupPSI parses a cgroup's memory.pressure file, which uses the
// same "some avg10=.. avg60=.. avg300=.. total=.." / "full ..." layout
// as /proc/pressure/memory. A missing file (PSI disabled, or a kernel
// too old to expose it per-cgroup) is (nil, nil), not an error.
func readCgroupPSI(path string) (*model.PSI, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is built from an operator-supplied sys root, not untrusted input
	if os.IsNotExist(err) {
		return nil, nil //nolint:nilnil // absence is a valid, expected state here
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var psi model.PSI
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 5 {
			continue
		}
		avg10, avg60, avg300, total, err := parsePSIKV(fields[1:])
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		switch fields[0] {
		case "some":
			psi.SomeAvg10, psi.SomeAvg60, psi.SomeAvg300, psi.SomeTotal = avg10, avg60, avg300, total
		case "full":
			psi.FullAvg10, psi.FullAvg60, psi.FullAvg300, psi.FullTotal = avg10, avg60, avg300, total
		}
	}
	return &psi, nil
}

// parsePSIKV parses the "avg10=.. avg60=.. avg300=.. total=.." fields
// of one memory.pressure line.
func parsePSIKV(fields []string) (avg10, avg60, avg300 float64, total uint64, err error) {
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		switch k {
		case "avg10":
			avg10, err = strconv.ParseFloat(v, 64)
		case "avg60":
			avg60, err = strconv.ParseFloat(v, 64)
		case "avg300":
			avg300, err = strconv.ParseFloat(v, 64)
		case "total":
			total, err = strconv.ParseUint(v, 10, 64)
		}
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}
	return avg10, avg60, avg300, total, nil
}
