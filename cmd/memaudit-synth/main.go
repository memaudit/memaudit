// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Command memaudit-synth is the synthetic workload used to prove DAMON's
// cold-page measurement is correct: it mmaps a fixed amount of memory and
// keeps touching a smaller hot subset, so a downstream damon_hist reading
// can be checked against a known-good cold-byte band.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	alloc := flag.String("alloc", "8GiB", "total memory to mmap")
	hot := flag.String("hot", "2GiB", "subset to keep touching")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "memaudit-synth --alloc=%s --hot=%s: not implemented yet\n", *alloc, *hot)
	os.Exit(1)
}
