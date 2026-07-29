// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package agent runs the tick loop: one goroutine per collector on a
// jittered ticker, writing to the spool, plus one ship-drain goroutine
// when shipping is configured.
package agent
