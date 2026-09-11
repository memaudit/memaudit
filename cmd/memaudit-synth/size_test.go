// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestParseSizeValid(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"8GiB", 8 * 1024 * 1024 * 1024},
		{"2GiB", 2 * 1024 * 1024 * 1024},
		{"512MiB", 512 * 1024 * 1024},
		{"1KiB", 1024},
		{"0GiB", 0},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if err != nil {
			t.Errorf("parseSize(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseSizeRejectsGarbage(t *testing.T) {
	cases := []string{"", "abc", "8XB", "GiB", "-8GiB", "8", "8 GiB"}
	for _, in := range cases {
		if _, err := parseSize(in); err == nil {
			t.Errorf("parseSize(%q): expected error, got nil", in)
		}
	}
}

func TestParseSizeRejectsOverflow(t *testing.T) {
	// 16777216 * 1TiB overflows uint64 back to exactly 0 if the
	// multiplication isn't guarded — must be rejected, not silently
	// wrapped into a small, wrong value.
	cases := []string{"16777216TiB", "17179869185GiB"}
	for _, in := range cases {
		got, err := parseSize(in)
		if err == nil {
			t.Errorf("parseSize(%q) = %d, want overflow error", in, got)
		}
	}
}
