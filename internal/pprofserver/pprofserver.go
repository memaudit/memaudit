// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package pprofserver is an opt-in, loopback-only net/http/pprof
// endpoint. It exists so a real leak or performance problem can be
// diagnosed on a running host — off by default, same as every other
// non-default-on setting in this project; an operator (or memaudit
// support, with the operator's cooperation) turns it on deliberately via
// debug.pprof_addr in config.yaml, never silently active.
package pprofserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: exactly the intended behavior here, guarded by ValidateAddr requiring a loopback address and this being opt-in — see package doc
	"time"
)

const shutdownTimeout = 5 * time.Second

// ValidateAddr checks that addr is safe to bind the debug endpoint to:
// an explicit loopback host (127.0.0.1, ::1, or localhost) with a port.
// Rejects an empty/wildcard host (e.g. ":6060", "0.0.0.0:6060") or any
// other host — this endpoint exposes runtime internals (goroutine
// stacks, heap contents) and must never be reachable from outside the
// host, regardless of what an operator's config typo might otherwise
// produce.
func ValidateAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid pprof address %q: %w", addr, err)
	}
	if port == "" {
		return fmt.Errorf("invalid pprof address %q: missing port", addr)
	}
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return nil
	default:
		return fmt.Errorf("invalid pprof address %q: host must be a loopback address (127.0.0.1, ::1, or localhost) — refusing to bind a debug endpoint to a non-loopback interface", addr)
	}
}

// Listen validates addr and opens a TCP listener on it.
func Listen(addr string) (net.Listener, error) {
	if err := ValidateAddr(addr); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	return ln, nil
}

// Serve exposes net/http/pprof on ln until ctx is cancelled, then shuts
// down gracefully (bounded by shutdownTimeout).
func Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: http.DefaultServeMux, ReadHeaderTimeout: shutdownTimeout}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
