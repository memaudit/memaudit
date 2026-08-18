// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"fmt"
	"os"
	"strings"
)

// writeSysfsFile writes value to a DAMON sysfs control file. Every write
// under kdamonds/admin is to a file the kernel already created (either at
// boot, or as a side effect of an earlier nr_* write in the same
// sequence) — a missing parent directory means the sequence wrote things
// out of order, and that's surfaced as a plain error, not retried or
// papered over.
func writeSysfsFile(path, value string) error {
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil { //nolint:gosec // G306: sysfs control files, not sensitive data
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func readSysfsFile(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is built from an operator-supplied sys root, not untrusted input
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}
