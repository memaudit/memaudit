// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

// Package model defines the wire format shared by every collector, the
// spool, and the shipper.
package model

import (
	"encoding/json"
	"time"
)

// Envelope is the single line written to the JSONL spool for every sample,
// regardless of record type.
type Envelope struct {
	TS      time.Time       `json:"ts"`
	Site    string          `json:"site"`
	Host    string          `json:"host"`
	Type    string          `json:"type"`
	Schema  int             `json:"schema"`
	Payload json.RawMessage `json:"payload"`
}
