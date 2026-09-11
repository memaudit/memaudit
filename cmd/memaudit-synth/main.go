// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Command memaudit-synth is the synthetic workload used to prove DAMON's
// cold-page measurement is correct: it mmaps a fixed amount of memory and
// keeps touching a smaller hot subset, so a downstream damon_hist reading
// can be checked against a known-good cold-byte band.
//
// It needs to run as root (DAMON's sysfs interface requires it, same as
// memauditd), and it needs to run alone on its kdamond slot — stop
// memauditd first if it's running on the same box.
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/memaudit/memaudit/internal/collector"
	"github.com/memaudit/memaudit/pkg/damon"
)

// version is set via -ldflags at release build time; "dev" for local
// builds, matching memauditd's convention.
var version = "dev"

// minDuration is a floor on --duration: cold_300s only becomes meaningful
// once a region has been continuously idle for 300s (see
// internal/collector.bucketRegions), so anything shorter always reports
// cold_300s=0 and fails for a reason that has nothing to do with DAMON's
// correctness. The extra minute over 300s leaves margin for the
// aggregation interval.
const minDuration = 6 * time.Minute

func main() {
	allocStr := flag.String("alloc", "8GiB", "total memory to mmap")
	hotStr := flag.String("hot", "2GiB", "contiguous subset to keep touching")
	duration := flag.Duration("duration", 10*time.Minute, "how long to run before checking the cold-byte band (minimum 6m)")
	force := flag.Bool("force", false, "skip the preflight and kdamond-in-use checks")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("memaudit-synth", version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *allocStr, *hotStr, *duration, *force); err != nil {
		fmt.Fprintln(os.Stderr, "memaudit-synth:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, allocStr, hotStr string, duration time.Duration, force bool) error {
	if duration < minDuration {
		return fmt.Errorf("--duration must be at least %s (cold_300s only becomes meaningful after 300s of continuous idle), got %s", minDuration, duration)
	}

	allocBytes, err := parseSize(allocStr)
	if err != nil {
		return fmt.Errorf("--alloc: %w", err)
	}
	hotBytes, err := parseSize(hotStr)
	if err != nil {
		return fmt.Errorf("--hot: %w", err)
	}
	if hotBytes == 0 {
		return fmt.Errorf("--hot must be greater than zero, got %s", hotStr)
	}
	if hotBytes > allocBytes {
		return fmt.Errorf("--hot (%s) exceeds --alloc (%s)", hotStr, allocStr)
	}
	if allocBytes > math.MaxInt {
		return fmt.Errorf("--alloc %s is too large to mmap on this platform", allocStr)
	}
	if hotBytes > math.MaxInt {
		return fmt.Errorf("--hot %s is too large to mmap on this platform", hotStr)
	}

	mi, err := collector.NewMeminfo("/proc").Collect()
	if err != nil {
		return fmt.Errorf("read /proc/meminfo: %w", err)
	}
	if err := checkPreflight(mi.MemTotal, allocBytes); err != nil {
		if !force {
			return fmt.Errorf("preflight check failed (pass --force to override): %w", err)
		}
		fmt.Fprintln(os.Stderr, "memaudit-synth: preflight check failed, continuing anyway (--force):", err)
	}

	caps, err := damon.Detect()
	if err != nil {
		return fmt.Errorf("damon.Detect (must run as root): %w", err)
	}
	if !caps.Sysfs {
		return fmt.Errorf("no DAMON sysfs interface on this kernel (need Linux 5.18+ with CONFIG_DAMON_SYSFS)")
	}

	running, err := kdamondAlreadyRunning("/sys")
	if err != nil {
		return fmt.Errorf("check kdamond state: %w", err)
	}
	if running {
		if !force {
			return fmt.Errorf("DAMON's kdamond 0 is already running (likely memauditd) — stop it first, or pass --force to take over its slot")
		}
		fmt.Fprintln(os.Stderr, "memaudit-synth: kdamond 0 is already running, taking over anyway (--force)")
	}

	fmt.Printf("memaudit-synth: mmapping %s, keeping %s hot for %s\n", allocStr, hotStr, duration)

	// MAP_ANON (not the Linux-only MAP_ANONYMOUS spelling) so this also
	// compiles on a non-Linux dev machine; DAMON itself is Linux-only, so
	// this tool only ever actually runs there regardless.
	region, err := syscall.Mmap(-1, 0, int(allocBytes), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		return fmt.Errorf("mmap %d bytes: %w", allocBytes, err)
	}
	defer func() {
		if err := syscall.Munmap(region); err != nil {
			fmt.Fprintln(os.Stderr, "memaudit-synth: munmap:", err)
		}
	}()

	// Touch the entire region once to force every page resident — an
	// untouched anonymous mapping has no physical backing yet, so DAMON
	// (which watches physical memory) would never see the "cold" 6GiB as
	// memory at all if it were left purely virtual.
	pageSize := os.Getpagesize()
	for i := 0; i < len(region); i += pageSize {
		region[i] = 1
	}

	// A fixed offset (not a randomized one) keeps this file simple to
	// read and trust: the hot subrange is always the first hotBytes of
	// the allocation.
	hotRegion := region[:hotBytes]

	stopHot := make(chan struct{})
	var hotWG sync.WaitGroup
	hotWG.Go(func() {
		touchHotLoop(hotRegion, pageSize, stopHot)
	})
	// Stopping the hot-touch goroutine must happen before Munmap runs (a
	// deferred call declared earlier than this one, so LIFO order runs it
	// after this) — writing to region after it's unmapped would segfault.
	defer func() {
		close(stopHot)
		hotWG.Wait()
	}()

	regions, err := damon.ParseIomem()
	if err != nil {
		return fmt.Errorf("ParseIomem (must run as root): %w", err)
	}

	const sampleUS, aggrUS, maxRegions = 5_000, 100_000, 1_000
	sess, err := damon.Start(damon.Config{
		Ops:        "paddr",
		SampleUS:   sampleUS,
		AggrUS:     aggrUS,
		UpdateUS:   1_000_000,
		MinRegions: 10,
		MaxRegions: maxRegions,
		Regions:    regions,
	})
	if err != nil {
		return fmt.Errorf("damon.Start (must run as root): %w", err)
	}
	defer func() { _ = sess.Stop() }()

	// tried_regions is a read-only sysfs output Start never touches, so
	// the only way to know the histogram readout actually works on this
	// kernel is to probe it for real, same as internal/agent/sources.go
	// does for the production agent.
	if _, err := sess.Snapshot(); err != nil {
		return fmt.Errorf("tried_regions probe failed (kernel may not support full histogram mode): %w", err)
	}

	dc := collector.NewDamon(sess, aggrUS)

	// Base the expected band on DAMON's own measured footprint
	// (MonitoredBytes, the real sum of "System RAM" ranges it's watching)
	// rather than /proc/meminfo's MemTotal used for preflight — the two
	// differ by firmware/kernel-reserved memory that preflight can't see
	// but DAMON does end up monitoring (and aging into cold_300s).
	// Clamped to allocBytes as a floor: if the process made it this far
	// (successfully mmap'd and touched the whole allocation), the real
	// monitored footprint should cover it; the clamp is only a safety net
	// against measurement-boundary oddities, not a path meant to rescue a
	// box that's actually too small — --force can't make physically
	// insufficient RAM sufficient.
	baseline, err := dc.Collect()
	if err != nil {
		return fmt.Errorf("collect baseline damon_hist: %w", err)
	}
	monitoredBytes := max(baseline.MonitoredBytes, allocBytes)
	low, high, err := bandBounds(allocBytes, hotBytes, monitoredBytes)
	if err != nil {
		return err
	}

	pollInterval := duration / 20
	if pollInterval > 30*time.Second {
		pollInterval = 30 * time.Second
	} else if pollInterval < time.Second {
		pollInterval = time.Second
	}

	start := time.Now()
	lastHist := baseline
	for elapsed := time.Duration(0); elapsed < duration; {
		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupted after %s, before completing --duration %s", time.Since(start).Round(time.Second), duration)
		case <-time.After(pollInterval):
		}
		elapsed = time.Since(start)

		hist, err := dc.Collect()
		if err != nil {
			return fmt.Errorf("collect damon_hist: %w", err)
		}
		lastHist = hist
		fmt.Printf("memaudit-synth: t=%s cold_300s=%s monitored=%s\n", elapsed.Round(time.Second), formatBytes(hist.Cold300s), formatBytes(hist.MonitoredBytes))
	}

	if lastHist.Cold300s < low || lastHist.Cold300s > high {
		return fmt.Errorf("FAIL: cold_300s=%s, want [%s, %s] (monitored=%s, machine_hot=%s)",
			formatBytes(lastHist.Cold300s), formatBytes(low), formatBytes(high), formatBytes(lastHist.MonitoredBytes), formatBytes(lastHist.HotBytes))
	}

	fmt.Printf("memaudit-synth: PASS: cold_300s=%s, want [%s, %s]\n", formatBytes(lastHist.Cold300s), formatBytes(low), formatBytes(high))
	return nil
}

// touchHotLoop writes across hotRegion in page-sized strides in a tight
// loop until stop is closed, keeping every page in it continuously
// accessed so DAMON never buckets it as cold.
func touchHotLoop(hotRegion []byte, pageSize int, stop <-chan struct{}) {
	i := 0
	for {
		select {
		case <-stop:
			return
		default:
			hotRegion[i%len(hotRegion)] = byte(i)
			i += pageSize
		}
	}
}
