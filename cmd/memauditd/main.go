// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Command memauditd is the memaudit host agent: it samples procfs/sysfs/
// cgroupfs, spools JSONL locally, and ships it to memaudit-ingest.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/memaudit/memaudit/internal/config"
)

var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	args := os.Args[1:]
	sub := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}

	switch sub {
	case "run":
		runCmd(args)
	case "selftest":
		selftestCmd(args)
	case "vllm-dump":
		vllmDumpCmd(args)
	case "version":
		fmt.Println("memauditd", version)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
		os.Exit(2)
	}
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/memaudit/config.yaml", "path to config.yaml")
	_ = fs.Parse(args) // ExitOnError: Parse never returns a non-nil error here

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	slog.Info("memauditd starting", "site", cfg.Site, "mode", cfg.Mode)
	slog.Warn("no collectors wired up yet — scaffold only")
}

func selftestCmd(_ []string) {
	fmt.Fprintln(os.Stderr, "memauditd selftest: not implemented yet")
	os.Exit(1)
}

func vllmDumpCmd(_ []string) {
	fmt.Fprintln(os.Stderr, "memauditd vllm-dump: not implemented yet")
	os.Exit(1)
}
