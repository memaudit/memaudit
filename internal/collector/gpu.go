// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/memaudit/memaudit/internal/model"
)

const gpuQueryFields = "index,uuid,name,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,mig.mode.current"
const computeAppsQueryFields = "gpu_uuid,pid,used_memory"

// gpuCollectTimeout bounds each nvidia-smi invocation — a hung driver
// shouldn't be able to stall a tick indefinitely.
const gpuCollectTimeout = 5 * time.Second

// GPU collects the gpu_mem record by exec'ing nvidia-smi and parsing its
// CSV output — no cgo, no in-process NVML. See package doc for why.
type GPU struct {
	binPath string
}

// NewGPU returns a GPU collector invoking binPath (normally
// "nvidia-smi", resolved via PATH; tests point it at a fixture script).
func NewGPU(binPath string) *GPU {
	return &GPU{binPath: binPath}
}

// Collect returns one record per GPU device. No nvidia-smi on PATH is a
// valid, expected state (nil, nil), not an error, matching every other
// collector's absence convention.
func (g *GPU) Collect() ([]model.GPUMem, error) {
	if _, err := exec.LookPath(g.binPath); err != nil {
		return nil, nil //nolint:nilnil // absence is a valid, expected state here
	}

	gpuOut, err := g.run("--query-gpu="+gpuQueryFields, "--format=csv,noheader,nounits")
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi query-gpu: %w", err)
	}
	devices, err := parseGPUQueryCSV(gpuOut)
	if err != nil {
		return nil, err
	}

	procsOut, err := g.run("--query-compute-apps="+computeAppsQueryFields, "--format=csv,noheader,nounits")
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi query-compute-apps: %w", err)
	}
	procsByUUID, err := parseComputeAppsCSV(procsOut)
	if err != nil {
		return nil, err
	}

	for i := range devices {
		devices[i].Processes = procsByUUID[devices[i].UUID]
	}
	return devices, nil
}

func (g *GPU) run(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gpuCollectTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, g.binPath, args...).Output() //nolint:gosec // G204: binPath/args are fixed, not user input
	if err != nil {
		return nil, err
	}
	return out, nil
}

// parseGPUQueryCSV parses `nvidia-smi --query-gpu=... --format=csv,noheader,nounits`
// output. Fields read [N/A] on some SKUs/drivers — tolerated per-field
// (reads as zero) rather than failing the whole record.
func parseGPUQueryCSV(data []byte) ([]model.GPUMem, error) {
	var out []model.GPUMem
	for _, line := range csvLines(data) {
		f, err := splitCSVFields(line, 9)
		if err != nil {
			return nil, fmt.Errorf("parse gpu query line %q: %w", line, err)
		}
		out = append(out, model.GPUMem{
			Index:      csvInt(f[0]),
			UUID:       f[1],
			Name:       f[2],
			MemTotal:   csvUint(f[3]),
			MemUsed:    csvUint(f[4]),
			MemFree:    csvUint(f[5]),
			UtilGPU:    csvUint32(f[6]),
			UtilMemory: csvUint32(f[7]),
			MIG:        f[8] == "Enabled",
		})
	}
	return out, nil
}

// parseComputeAppsCSV parses `nvidia-smi --query-compute-apps=... --format=csv,noheader,nounits`
// output, grouped by the owning GPU's UUID.
func parseComputeAppsCSV(data []byte) (map[string][]model.GPUProcess, error) {
	out := map[string][]model.GPUProcess{}
	for _, line := range csvLines(data) {
		f, err := splitCSVFields(line, 3)
		if err != nil {
			return nil, fmt.Errorf("parse compute-apps line %q: %w", line, err)
		}
		uuid := f[0]
		out[uuid] = append(out[uuid], model.GPUProcess{
			PID:        csvUint32(f[1]),
			UsedMemory: csvUint(f[2]),
		})
	}
	return out, nil
}

func csvLines(data []byte) []string {
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func splitCSVFields(line string, want int) ([]string, error) {
	fields := strings.Split(line, ",")
	if len(fields) != want {
		return nil, fmt.Errorf("got %d fields, want %d", len(fields), want)
	}
	for i, f := range fields {
		fields[i] = strings.TrimSpace(f)
	}
	return fields, nil
}

// csvUint parses a numeric CSV field, tolerating nvidia-smi's [N/A]
// sentinel (and any other non-numeric value) as zero rather than erroring
// the whole record.
func csvUint(s string) uint64 {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// csvUint32 is csvUint narrowed to uint32 (GPU utilization percentages,
// PIDs), with an explicit upper-bound check before the conversion rather
// than relying on the wraparound that a bare uint32(csvUint(s)) risks on
// a malformed or hostile field.
func csvUint32(s string) uint32 {
	n := csvUint(s)
	if n > math.MaxUint32 {
		return 0
	}
	return uint32(n)
}

// csvInt is csvUint narrowed to int (a GPU index), same explicit-bound
// reasoning as csvUint32.
func csvInt(s string) int {
	n := csvUint(s)
	if n > math.MaxInt {
		return 0
	}
	return int(n)
}
