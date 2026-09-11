// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestCheckPreflightAcceptsSuitableBox(t *testing.T) {
	cases := []struct {
		name       string
		totalBytes uint64
		allocBytes uint64
	}{
		{"total at exactly alloc + min headroom (inclusive lower bound)", 8*gib + minHeadroomBytes, 8 * gib},
		{"total at 1.2x alloc", 8*gib + 8*gib/5, 8 * gib},
		{"total at 2x alloc (inclusive upper bound: ambient == alloc)", 16 * gib, 8 * gib},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := checkPreflight(c.totalBytes, c.allocBytes); err != nil {
				t.Errorf("checkPreflight(%d, %d): unexpected error: %v", c.totalBytes, c.allocBytes, err)
			}
		})
	}
}

func TestCheckPreflightRejectsUnsuitableBox(t *testing.T) {
	cases := []struct {
		name       string
		totalBytes uint64
		allocBytes uint64
	}{
		{"total below alloc", 6 * gib, 8 * gib},
		{"total equals alloc (zero headroom, OOM risk)", 8 * gib, 8 * gib},
		{"total just under min headroom", 8*gib + minHeadroomBytes - 1, 8 * gib},
		{"total just above 2x alloc", 16*gib + 1, 8 * gib},
		{"total far above alloc (busy production box)", 64 * gib, 8 * gib},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := checkPreflight(c.totalBytes, c.allocBytes); err == nil {
				t.Errorf("checkPreflight(%d, %d): expected error, got nil", c.totalBytes, c.allocBytes)
			}
		})
	}
}
