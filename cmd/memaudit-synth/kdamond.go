// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// kdamondStateRelPath is DAMON's single kdamond slot, relative to a sys
// root — the same one pkg/damon.Start always configures (index 0), and the
// same one memauditd's own DAMON collector uses.
const kdamondStateRelPath = "kernel/mm/damon/admin/kdamonds/0/state"

// kdamondStateIsOn reports whether raw (the contents of a kdamond's sysfs
// "state" file) indicates it's currently monitoring.
func kdamondStateIsOn(raw string) bool {
	return strings.TrimSpace(raw) == "on"
}

// kdamondAlreadyRunning reports whether kdamond 0 is currently on under
// sysRoot (normally "/sys"; tests point it at a fixture directory) —
// meaning something else (most likely memauditd) already owns DAMON's one
// monitoring slot. Absence of the file (no kdamond configured yet) is not
// an error: it just means nothing is running.
func kdamondAlreadyRunning(sysRoot string) (bool, error) {
	b, err := os.ReadFile(filepath.Join(sysRoot, kdamondStateRelPath)) //nolint:gosec // G304: sysRoot is operator-supplied ("/sys" in production, a fixture dir in tests), not untrusted input
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return kdamondStateIsOn(string(b)), nil
}
