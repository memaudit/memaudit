// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"encoding/json"
	"os"
	"testing"
)

// assertGoldenJSON compares got against the JSON fixture at wantPath,
// normalizing both through an unmarshal/marshal round trip so field order
// and formatting differences don't cause spurious failures.
func assertGoldenJSON(t *testing.T, wantPath string, got any) {
	t.Helper()

	wantRaw, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read %s: %v", wantPath, err)
	}
	var want any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatalf("unmarshal %s: %v", wantPath, err)
	}

	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	var gotDecoded any
	if err := json.Unmarshal(gotRaw, &gotDecoded); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}

	wantPretty, _ := json.MarshalIndent(want, "", "  ")
	gotPretty, _ := json.MarshalIndent(gotDecoded, "", "  ")
	if string(wantPretty) != string(gotPretty) {
		t.Errorf("mismatch against %s\n--- want ---\n%s\n--- got ---\n%s", wantPath, wantPretty, gotPretty)
	}
}
