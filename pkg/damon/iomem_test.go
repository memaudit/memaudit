// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package damon

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestParseIomemFileGolden(t *testing.T) {
	f, err := os.Open("../../testdata/fedora-damon/proc/iomem")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	got, err := parseIomemReader(f)
	if err != nil {
		t.Fatalf("parseIomemReader: %v", err)
	}

	// Only top-level "System RAM" lines count; indented sub-ranges (PCI
	// buses, kernel code/rodata/data/bss under the third range, etc.) are
	// subdivisions of a System RAM range, not separate regions.
	want := []AddrRange{
		{Start: 0x1000, End: 0x9fbff},
		{Start: 0x100000, End: 0x7ffdbfff},
		{Start: 0x100000000, End: 0x179ffffff},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseIomemNonRootReturnsError(t *testing.T) {
	// Unprivileged reads of /proc/iomem mask every address to
	// 00000000-00000000, labels intact. A real "System RAM" range can
	// never legitimately be a zero-sized region at address 0, so that
	// pattern is the non-root signal.
	f, err := os.Open("../../testdata/edge-cases/iomem-non-root/proc/iomem")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	_, err = parseIomemReader(f)
	if err == nil {
		t.Fatal("parseIomemReader: got nil error, want an error about masked addresses")
	}
	if !errors.Is(err, ErrIomemMasked) {
		t.Fatalf("got error %v, want errors.Is(err, ErrIomemMasked)", err)
	}
}
