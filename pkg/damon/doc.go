// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package damon is a standalone Go client for the DAMON sysfs interface
// under /sys/kernel/mm/damon/admin/. It is importable independently of the
// rest of memaudit.
//
// Detect, ParseIomem, Start, and Stop are implemented. Snapshot (reading
// back tried_regions) and histogram bucketing are not yet — they land
// alongside the agent-side DAMON collector.
package damon
