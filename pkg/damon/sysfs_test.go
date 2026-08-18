// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"path/filepath"
	"testing"
)

func TestWriteSysfsFileThenReadSysfsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := writeSysfsFile(path, "on"); err != nil {
		t.Fatalf("writeSysfsFile: %v", err)
	}
	got, err := readSysfsFile(path)
	if err != nil {
		t.Fatalf("readSysfsFile: %v", err)
	}
	if got != "on" {
		t.Fatalf("got %q, want %q", got, "on")
	}
}

func TestWriteSysfsFileMissingDirReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "state")
	if err := writeSysfsFile(path, "on"); err == nil {
		t.Fatal("writeSysfsFile: got nil error, want one (parent dir doesn't exist, same as a kernel rejecting a write to a not-yet-created sysfs node)")
	}
}
