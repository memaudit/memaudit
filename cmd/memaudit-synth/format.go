// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

// formatBytes renders b as a GiB figure for progress/result output.
func formatBytes(b uint64) string {
	return fmt.Sprintf("%.2fGiB", float64(b)/float64(gib))
}
