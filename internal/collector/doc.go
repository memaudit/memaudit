// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package collector holds one file per record type: host memory, vmstat,
// PSI, NUMA, cgroups, hugepages, DAMON histograms, GPU memory, and vLLM
// metrics. Cgroups, hugepages, DAMON, GPU, and vLLM are not implemented
// yet.
package collector
