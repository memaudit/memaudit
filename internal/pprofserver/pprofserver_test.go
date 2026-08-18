// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package pprofserver

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestValidateAddrAcceptsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:6060", "localhost:6060", "[::1]:6060"} {
		if err := ValidateAddr(addr); err != nil {
			t.Errorf("ValidateAddr(%q): got %v, want nil", addr, err)
		}
	}
}

func TestValidateAddrRejectsWildcardOrNonLoopback(t *testing.T) {
	// Every one of these would make the debug endpoint reachable from
	// somewhere other than this host -- exactly what must never happen
	// for an endpoint that exposes runtime internals (goroutine stacks,
	// heap contents).
	for _, addr := range []string{
		":6060",            // empty host = all interfaces
		"0.0.0.0:6060",     // explicit wildcard
		"[::]:6060",        // explicit IPv6 wildcard
		"192.168.1.5:6060", // a real (non-loopback) host address
		"example.com:6060", // an arbitrary hostname
	} {
		if err := ValidateAddr(addr); err == nil {
			t.Errorf("ValidateAddr(%q): got nil error, want a rejection", addr)
		}
	}
}

func TestValidateAddrRejectsMissingPort(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "127.0.0.1:"} {
		if err := ValidateAddr(addr); err == nil {
			t.Errorf("ValidateAddr(%q): got nil error, want a rejection (no port)", addr)
		}
	}
}

func TestListenRejectsNonLoopback(t *testing.T) {
	if _, err := Listen("0.0.0.0:6060"); err == nil {
		t.Fatal("Listen(\"0.0.0.0:6060\"): got nil error, want a rejection")
	}
}

func TestListenAndServeExposesPprofThenShutsDownCleanly(t *testing.T) {
	ln, err := Listen("127.0.0.1:0") // :0 = OS picks a free port
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln) }()

	// Poll briefly for the server to come up rather than a fixed sleep.
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get(fmt.Sprintf("http://%s/debug/pprof/", addr))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /debug/pprof/: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/pprof/: status = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned error after shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return within 3s of context cancellation")
	}
}
