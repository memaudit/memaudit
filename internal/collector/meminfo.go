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

	"github.com/prometheus/procfs"

	"github.com/memaudit/memaudit/internal/model"
)

// Meminfo collects the host_mem record from /proc/meminfo.
type Meminfo struct {
	procRoot string
}

// NewMeminfo returns a Meminfo collector rooted at procRoot (normally
// "/proc"; tests point it at a fixture directory).
func NewMeminfo(procRoot string) *Meminfo {
	return &Meminfo{procRoot: procRoot}
}

// Collect reads and parses /proc/meminfo.
func (m *Meminfo) Collect() (*model.HostMem, error) {
	fs, err := procfs.NewFS(m.procRoot)
	if err != nil {
		return nil, fmt.Errorf("open proc root %s: %w", m.procRoot, err)
	}
	mi, err := fs.Meminfo()
	if err != nil {
		return nil, fmt.Errorf("read meminfo: %w", err)
	}

	// prometheus/procfs (as of v0.21.1) doesn't expose KReclaimable as a
	// struct field, so it's parsed directly off the same file.
	kreclaimable, err := readKReclaimableBytes(filepath.Join(m.procRoot, "meminfo"))
	if err != nil {
		return nil, err
	}

	return &model.HostMem{
		MemTotal:     derefU64(mi.MemTotalBytes),
		MemFree:      derefU64(mi.MemFreeBytes),
		MemAvailable: derefU64(mi.MemAvailableBytes),
		Buffers:      derefU64(mi.BuffersBytes),
		Cached:       derefU64(mi.CachedBytes),
		SwapCached:   derefU64(mi.SwapCachedBytes),
		SwapTotal:    derefU64(mi.SwapTotalBytes),
		SwapFree:     derefU64(mi.SwapFreeBytes),
		Active:       derefU64(mi.ActiveBytes),
		Inactive:     derefU64(mi.InactiveBytes),
		ActiveAnon:   derefU64(mi.ActiveAnonBytes),
		InactiveAnon: derefU64(mi.InactiveAnonBytes),
		ActiveFile:   derefU64(mi.ActiveFileBytes),
		InactiveFile: derefU64(mi.InactiveFileBytes),
		Unevictable:  derefU64(mi.UnevictableBytes),
		Dirty:        derefU64(mi.DirtyBytes),
		Writeback:    derefU64(mi.WritebackBytes),
		AnonPages:    derefU64(mi.AnonPagesBytes),
		Mapped:       derefU64(mi.MappedBytes),
		Shmem:        derefU64(mi.ShmemBytes),
		KReclaimable: kreclaimable,
		Slab:         derefU64(mi.SlabBytes),
		SReclaimable: derefU64(mi.SReclaimableBytes),
		SUnreclaim:   derefU64(mi.SUnreclaimBytes),
		KernelStack:  derefU64(mi.KernelStackBytes),
		PageTables:   derefU64(mi.PageTablesBytes),
		CommitLimit:  derefU64(mi.CommitLimitBytes),
		CommittedAS:  derefU64(mi.CommittedASBytes),
		DirectMap4k:  derefU64(mi.DirectMap4kBytes),
		DirectMap2M:  derefU64(mi.DirectMap2MBytes),
		DirectMap1G:  derefU64(mi.DirectMap1GBytes),
	}, nil
}

func derefU64(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

func readKReclaimableBytes(path string) (uint64, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path is built from an operator-supplied proc root, not untrusted input
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "KReclaimable:" {
			continue
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse KReclaimable: %w", err)
		}
		return kb * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan %s: %w", path, err)
	}
	// KReclaimable is absent on pre-4.20 kernels; zero, not an error.
	return 0, nil
}
