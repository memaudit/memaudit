// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Command memauditd is the memaudit host agent: it samples procfs/sysfs/
// cgroupfs, spools JSONL locally, and ships it to memaudit-ingest.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/memaudit/memaudit/internal/agent"
	"github.com/memaudit/memaudit/internal/config"
	"github.com/memaudit/memaudit/internal/k8s"
	"github.com/memaudit/memaudit/internal/selftest"
	"github.com/memaudit/memaudit/internal/ship"
	"github.com/memaudit/memaudit/internal/spool"
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

	host, err := os.Hostname()
	if err != nil {
		slog.Error("get hostname", "err", err)
		os.Exit(1)
	}

	sp, err := spool.Open(cfg.Spool.Dir, spool.Options{
		MaxBytes: cfg.Spool.MaxBytes,
		Site:     cfg.Site,
		Host:     host,
	})
	if err != nil {
		slog.Error("open spool", "err", err)
		os.Exit(1)
	}

	var shipper *ship.Shipper
	if cfg.Ship.Mode == "push" {
		// Without a URL every request fails with an unsupported-scheme
		// error, which the shipper classifies as retryable: it would back
		// off to its ceiling and retry forever while the spool filled up
		// and began evicting. Say so here, where an operator can see it.
		if cfg.Ship.URL == "" {
			slog.Error("ship.mode is push but ship.url is empty; set ship.url or switch to ship.mode: bundle")
			os.Exit(1)
		}
		token, err := readToken(cfg.Ship.TokenFile)
		if err != nil {
			slog.Error("read ship token", "err", err)
			os.Exit(1)
		}
		shipper = ship.New(ship.Config{URL: cfg.Ship.URL, Token: token})
	}

	var k8sClient k8s.Client
	if cfg.K8s.Enrich {
		token, err := readToken(cfg.K8s.TokenPath)
		if err != nil {
			slog.Error("read k8s token", "err", err)
			os.Exit(1)
		}
		kc, err := k8s.NewKubeletClient(k8s.KubeletClientConfig{
			BaseURL:            cfg.K8s.Kubelet,
			Token:              token,
			CAPath:             cfg.K8s.CAPath,
			InsecureSkipVerify: cfg.K8s.InsecureSkipVerify,
			LabelKeys:          cfg.K8s.LabelKeys,
		})
		if err != nil {
			slog.Error("build kubelet client", "err", err)
			os.Exit(1)
		}
		k8sClient = kc
	}

	slog.Info("memauditd starting", "site", cfg.Site, "mode", cfg.Mode, "ship_mode", cfg.Ship.Mode)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := agent.Run(ctx, agent.Config{
		Site:       cfg.Site,
		Host:       host,
		ProcRoot:   "/proc",
		SysRoot:    "/sys",
		Interval:   time.Duration(cfg.IntervalS) * time.Second,
		Collectors: cfg.Collectors,
		K8sClient:  k8sClient,
		Spool:      sp,
		Shipper:    shipper,
	}); err != nil {
		// The final ship drain is best-effort: by the time it runs the
		// spool is closed and durably on disk, and a normal host shutdown
		// routinely tears down networking underneath it. Exiting non-zero
		// there would mark the unit failed over data that is safe and
		// ships on the next start.
		if errors.Is(err, agent.ErrShutdownDrainIncomplete) {
			slog.Warn("shutdown ship drain incomplete; spool is on disk and will ship on next start", "err", err)
			return
		}
		slog.Error("agent run", "err", err)
		os.Exit(1)
	}
}

// readToken reads path (a token file referenced from config), trimming
// trailing whitespace. Empty path means "no token configured" and
// returns "" without error.
func readToken(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304: path is operator-supplied via config, not untrusted input
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func selftestCmd(_ []string) {
	res := selftest.Run("/proc", "/sys")
	fmt.Print(res.String())
	if res.Failed() {
		os.Exit(1)
	}
}
