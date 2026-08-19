// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"os"
	"reflect"
	"testing"

	"github.com/memaudit/memaudit/pkg/model"
)

func TestParseGPUQueryCSVGolden(t *testing.T) {
	csv := "0, GPU-aaaaaaaa-0000-0000-0000-000000000000, NVIDIA H100 80GB HBM3, 81920, 1024, 80896, 5, 2, Disabled\n" +
		"1, GPU-bbbbbbbb-0000-0000-0000-000000000000, NVIDIA H100 80GB HBM3, 81920, 0, 81920, 0, 0, Enabled\n"

	got, err := parseGPUQueryCSV([]byte(csv))
	if err != nil {
		t.Fatalf("parseGPUQueryCSV: %v", err)
	}
	want := []model.GPUMem{
		{
			Index: 0, UUID: "GPU-aaaaaaaa-0000-0000-0000-000000000000", Name: "NVIDIA H100 80GB HBM3",
			MemTotal: 81920, MemUsed: 1024, MemFree: 80896, UtilGPU: 5, UtilMemory: 2, MIG: false,
		},
		{
			Index: 1, UUID: "GPU-bbbbbbbb-0000-0000-0000-000000000000", Name: "NVIDIA H100 80GB HBM3",
			MemTotal: 81920, MemUsed: 0, MemFree: 81920, UtilGPU: 0, UtilMemory: 0, MIG: true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseGPUQueryCSVEmptyIsNilNotError(t *testing.T) {
	// No GPUs (or nvidia-smi ran but found none): empty output, valid
	// state, not an error.
	got, err := parseGPUQueryCSV([]byte(""))
	if err != nil {
		t.Fatalf("parseGPUQueryCSV: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}

func TestParseGPUQueryCSVNATolerantPerField(t *testing.T) {
	// [N/A] on individual fields (seen on some SKUs/drivers, e.g.
	// utilization on certain vGPU profiles): the field reads as zero,
	// the rest of the record is still usable, not a hard error.
	csv := "0, GPU-aaaaaaaa-0000-0000-0000-000000000000, NVIDIA T4, 16384, 512, 15872, [N/A], [N/A], [N/A]\n"

	got, err := parseGPUQueryCSV([]byte(csv))
	if err != nil {
		t.Fatalf("parseGPUQueryCSV: %v", err)
	}
	want := []model.GPUMem{
		{
			Index: 0, UUID: "GPU-aaaaaaaa-0000-0000-0000-000000000000", Name: "NVIDIA T4",
			MemTotal: 16384, MemUsed: 512, MemFree: 15872, UtilGPU: 0, UtilMemory: 0, MIG: false,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseComputeAppsCSVGolden(t *testing.T) {
	csv := "GPU-aaaaaaaa-0000-0000-0000-000000000000, 1234, 2048\n" +
		"GPU-aaaaaaaa-0000-0000-0000-000000000000, 5678, 4096\n" +
		"GPU-bbbbbbbb-0000-0000-0000-000000000000, 9012, 1024\n"

	got, err := parseComputeAppsCSV([]byte(csv))
	if err != nil {
		t.Fatalf("parseComputeAppsCSV: %v", err)
	}
	want := map[string][]model.GPUProcess{
		"GPU-aaaaaaaa-0000-0000-0000-000000000000": {
			{PID: 1234, UsedMemory: 2048},
			{PID: 5678, UsedMemory: 4096},
		},
		"GPU-bbbbbbbb-0000-0000-0000-000000000000": {
			{PID: 9012, UsedMemory: 1024},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseComputeAppsCSVEmptyIsNilNotError(t *testing.T) {
	// No compute processes running: valid state, not an error.
	got, err := parseComputeAppsCSV([]byte(""))
	if err != nil {
		t.Fatalf("parseComputeAppsCSV: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}

func TestGPUCollectAbsentBinaryIsNilNotError(t *testing.T) {
	got, err := NewGPU("no-such-binary-anywhere-on-path").Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

// TestParseGPUQueryCSVRealCapture and TestParseComputeAppsCSVRealCapture
// use a real capture from a rented 2x NVIDIA A40 box (RunPod), not a
// hand-authored fixture — see testdata/README.md. This is what confirmed
// (rather than assumed) that mig.mode.current reads "[N/A]" on
// non-MIG-capable cards, not just "Enabled"/"Disabled", and that the
// gpu_uuid field in --query-compute-apps really does correlate a process
// back to its actual owning device on real multi-GPU hardware — the one
// assumption that couldn't be verified any other way.
func TestParseGPUQueryCSVRealCapture(t *testing.T) {
	data, err := os.ReadFile("../../testdata/runpod-a40-x2/query-gpu.csv")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got, err := parseGPUQueryCSV(data)
	if err != nil {
		t.Fatalf("parseGPUQueryCSV: %v", err)
	}
	want := []model.GPUMem{
		{Index: 0, UUID: "GPU-a455e61c-8dc7-8df5-4130-9f82faf18cc9", Name: "NVIDIA A40", MemTotal: 46068, MemUsed: 335, MemFree: 45154, UtilGPU: 0, UtilMemory: 0, MIG: false},
		{Index: 1, UUID: "GPU-02e982b7-d7af-d91f-5aa5-d72083884148", Name: "NVIDIA A40", MemTotal: 46068, MemUsed: 335, MemFree: 45154, UtilGPU: 0, UtilMemory: 0, MIG: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseComputeAppsCSVRealCaptureCorrelatesByDevice(t *testing.T) {
	data, err := os.ReadFile("../../testdata/runpod-a40-x2/query-compute-apps.csv")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got, err := parseComputeAppsCSV(data)
	if err != nil {
		t.Fatalf("parseComputeAppsCSV: %v", err)
	}
	// Real, distinguishable evidence: a process started on cuda:0 (PID
	// 6643) correlates with device 0's UUID, and a process started on
	// cuda:1 (PID 6644) correlates with device 1's — cross-checked
	// against the same session's --query-gpu output.
	want := map[string][]model.GPUProcess{
		"GPU-a455e61c-8dc7-8df5-4130-9f82faf18cc9": {{PID: 6643, UsedMemory: 326}},
		"GPU-02e982b7-d7af-d91f-5aa5-d72083884148": {{PID: 6644, UsedMemory: 326}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
