// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKdamondAlreadyRunningFixtures(t *testing.T) {
	cases := []struct {
		name    string
		content string // "" means: don't create the file at all
		want    bool
	}{
		{"kdamond on", "on\n", true},
		{"kdamond off", "off\n", false},
		{"no kdamond configured yet", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sysRoot := t.TempDir()
			if c.content != "" {
				statePath := filepath.Join(sysRoot, "kernel/mm/damon/admin/kdamonds/0/state")
				if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(statePath, []byte(c.content), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			got, err := kdamondAlreadyRunning(sysRoot)
			if err != nil {
				t.Fatalf("kdamondAlreadyRunning(%q): unexpected error: %v", sysRoot, err)
			}
			if got != c.want {
				t.Errorf("kdamondAlreadyRunning(%q) = %v, want %v", sysRoot, got, c.want)
			}
		})
	}
}

func TestKdamondStateIsOn(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"on\n", true},
		{"on", true},
		{"off\n", false},
		{"off", false},
		{"", false},
	}
	for _, c := range cases {
		if got := kdamondStateIsOn(c.raw); got != c.want {
			t.Errorf("kdamondStateIsOn(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}
