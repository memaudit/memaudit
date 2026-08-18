// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package model defines the wire format shared by every collector, the
// spool, and the shipper.
package model

import (
	"encoding/json"
	"time"
)

// Envelope is the single line written to the JSONL spool for every sample,
// regardless of record type.
type Envelope struct {
	TS      time.Time       `json:"ts"`
	Site    string          `json:"site"`
	Host    string          `json:"host"`
	Type    string          `json:"type"`
	Schema  int             `json:"schema"`
	Payload json.RawMessage `json:"payload"`
}

// HostMem is the host_mem payload, sourced from /proc/meminfo. All fields
// are bytes.
type HostMem struct {
	MemTotal     uint64 `json:"mem_total"`
	MemFree      uint64 `json:"mem_free"`
	MemAvailable uint64 `json:"mem_available"`
	Buffers      uint64 `json:"buffers"`
	Cached       uint64 `json:"cached"`
	SwapCached   uint64 `json:"swap_cached"`
	SwapTotal    uint64 `json:"swap_total"`
	SwapFree     uint64 `json:"swap_free"`
	Active       uint64 `json:"active"`
	Inactive     uint64 `json:"inactive"`
	ActiveAnon   uint64 `json:"active_anon"`
	InactiveAnon uint64 `json:"inactive_anon"`
	ActiveFile   uint64 `json:"active_file"`
	InactiveFile uint64 `json:"inactive_file"`
	Unevictable  uint64 `json:"unevictable"`
	Dirty        uint64 `json:"dirty"`
	Writeback    uint64 `json:"writeback"`
	AnonPages    uint64 `json:"anon_pages"`
	Mapped       uint64 `json:"mapped"`
	Shmem        uint64 `json:"shmem"`
	KReclaimable uint64 `json:"kreclaimable"`
	Slab         uint64 `json:"slab"`
	SReclaimable uint64 `json:"slab_reclaimable"`
	SUnreclaim   uint64 `json:"slab_unreclaimable"`
	KernelStack  uint64 `json:"kernel_stack"`
	PageTables   uint64 `json:"page_tables"`
	CommitLimit  uint64 `json:"commit_limit"`
	CommittedAS  uint64 `json:"committed_as"`
	DirectMap4k  uint64 `json:"direct_map4k"`
	DirectMap2M  uint64 `json:"direct_map2m"`
	DirectMap1G  uint64 `json:"direct_map1g"`
}

// Vmstat is the vmstat payload: selected cumulative counters from
// /proc/vmstat. Deltas are computed downstream in SQL, never here.
type Vmstat struct {
	PgscanKswapd           uint64 `json:"pgscan_kswapd"`
	PgscanDirect           uint64 `json:"pgscan_direct"`
	PgstealKswapd          uint64 `json:"pgsteal_kswapd"`
	PgstealDirect          uint64 `json:"pgsteal_direct"`
	Pswpin                 uint64 `json:"pswpin"`
	Pswpout                uint64 `json:"pswpout"`
	WorkingsetRefaultAnon  uint64 `json:"workingset_refault_anon"`
	WorkingsetRefaultFile  uint64 `json:"workingset_refault_file"`
	WorkingsetActivateAnon uint64 `json:"workingset_activate_anon"`
	WorkingsetActivateFile uint64 `json:"workingset_activate_file"`
	OomKill                uint64 `json:"oom_kill"`
	PgmajFault             uint64 `json:"pgmajfault"`
}

// PSI is the psi payload, sourced from /proc/pressure/memory. Avg fields
// are percentages over the named window; Total is cumulative stall time in
// microseconds.
type PSI struct {
	SomeAvg10  float64 `json:"some_avg10"`
	SomeAvg60  float64 `json:"some_avg60"`
	SomeAvg300 float64 `json:"some_avg300"`
	SomeTotal  uint64  `json:"some_total"`
	FullAvg10  float64 `json:"full_avg10"`
	FullAvg60  float64 `json:"full_avg60"`
	FullAvg300 float64 `json:"full_avg300"`
	FullTotal  uint64  `json:"full_total"`
}

// NumaMem is one numa_mem payload per NUMA node, sourced from
// /sys/devices/system/node/nodeN/{meminfo,numastat}.
type NumaMem struct {
	Node        int    `json:"node"`
	MemTotal    uint64 `json:"mem_total"`
	MemFree     uint64 `json:"mem_free"`
	FilePages   uint64 `json:"file_pages"`
	AnonPages   uint64 `json:"anon_pages"`
	NumaHit     uint64 `json:"numa_hit"`
	NumaMiss    uint64 `json:"numa_miss"`
	NumaForeign uint64 `json:"numa_foreign"`
}

// HugepagesNoNUMA is the Node value used when a host has no per-node
// hugepages sysfs (single-node hosts, some containers): the record
// carries host-level totals instead of a per-node breakdown.
const HugepagesNoNUMA = -1

// Hugepages is one hugepages payload per (page size, NUMA node), sourced
// from /sys/kernel/mm/hugepages and, when present,
// /sys/devices/system/node/nodeN/hugepages. Total/Free/Rsvd/Surp are page
// counts, not bytes.
type Hugepages struct {
	SizeKB uint64 `json:"size_kb"`
	Node   int    `json:"node"`
	Total  uint64 `json:"total"`
	Free   uint64 `json:"free"`
	Rsvd   uint64 `json:"rsvd"`
	Surp   uint64 `json:"surp"`
}

// CgroupMem is one cgroup_mem payload per selected cgroup v2 directory,
// sourced from memory.current/max/min/low/high/peak/swap.current,
// memory.pressure, and memory.stat under /sys/fs/cgroup. Nullable fields
// are nil when the kernel omits the file (e.g. memory.peak on <5.19) or
// the file reads "max" (no limit set).
type CgroupMem struct {
	Cgroup string `json:"cgroup"`

	Current     uint64  `json:"current"`
	Max         *uint64 `json:"max,omitempty"`
	Min         *uint64 `json:"min,omitempty"`
	Low         *uint64 `json:"low,omitempty"`
	High        *uint64 `json:"high,omitempty"`
	Peak        *uint64 `json:"peak,omitempty"`
	SwapCurrent uint64  `json:"swap_current"`

	// PSI is nil when the cgroup has no memory.pressure file (PSI
	// disabled, or a kernel too old to expose it per-cgroup).
	PSI *PSI `json:"psi,omitempty"`

	Anon                   uint64 `json:"anon"`
	File                   uint64 `json:"file"`
	Kernel                 uint64 `json:"kernel"`
	SlabReclaimable        uint64 `json:"slab_reclaimable"`
	SlabUnreclaimable      uint64 `json:"slab_unreclaimable"`
	FileMapped             uint64 `json:"file_mapped"`
	FileDirty              uint64 `json:"file_dirty"`
	InactiveAnon           uint64 `json:"inactive_anon"`
	ActiveAnon             uint64 `json:"active_anon"`
	InactiveFile           uint64 `json:"inactive_file"`
	ActiveFile             uint64 `json:"active_file"`
	WorkingsetRefaultAnon  uint64 `json:"workingset_refault_anon"`
	WorkingsetRefaultFile  uint64 `json:"workingset_refault_file"`
	WorkingsetActivateAnon uint64 `json:"workingset_activate_anon"`
	WorkingsetActivateFile uint64 `json:"workingset_activate_file"`
	Pgscan                 uint64 `json:"pgscan"`
	Pgsteal                uint64 `json:"pgsteal"`

	// K8s enrichment fields, populated only when k8s.enrich is on and
	// the cgroup's pod UID resolves via the kubelet. Zero-valued
	// otherwise.
	K8sNamespace   string            `json:"k8s_ns,omitempty"`
	K8sPod         string            `json:"k8s_pod,omitempty"`
	K8sContainerID string            `json:"k8s_container_id,omitempty"`
	K8sLabels      map[string]string `json:"k8s_labels,omitempty"`
}

// DamonHist is a pre-bucketed cold-page histogram derived from a DAMON
// snapshot's regions — never raw regions, whose cardinality would be
// unbounded. Buckets are cumulative: a region cold for Cold24h also
// counts toward every smaller bucket.
type DamonHist struct {
	MonitoredBytes uint64 `json:"monitored_bytes"`
	HotBytes       uint64 `json:"hot_bytes"`
	Cold60s        uint64 `json:"cold_60s"`
	Cold300s       uint64 `json:"cold_300s"`
	Cold1h         uint64 `json:"cold_1h"`
	Cold6h         uint64 `json:"cold_6h"`
	Cold24h        uint64 `json:"cold_24h"`
}
