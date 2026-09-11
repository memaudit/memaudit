# memaudit

[![ci](https://github.com/memaudit/memaudit/actions/workflows/ci.yaml/badge.svg)](https://github.com/memaudit/memaudit/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/memaudit/memaudit.svg)](https://github.com/memaudit/memaudit/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/memaudit/memaudit.svg)](https://pkg.go.dev/github.com/memaudit/memaudit)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A read-only host agent for measuring cold (idle) memory, stranded DRAM, and
KV-cache waste on Linux hosts. Uses DAMON, cgroup v2, and PSI. No eBPF, no
kernel modules, just procfs/sysfs/cgroupfs.

This repo has the agent (`memauditd`), the DAMON client (`pkg/damon`), a
synthetic-workload correctness check (`memaudit-synth`), and deployment
files. The agent runs standalone in bundle mode too: local JSONL, no server
needed.

> [!IMPORTANT]
> Pre-1.0 software: config format, CLI flags, and wire format may change
> without notice before v1.0.0. Pin a specific release if that matters to
> you (`install.sh --version vX.Y.Z`).

`memauditd` collects host memory (meminfo, vmstat, PSI, NUMA, cgroup v2,
hugepages, optional Kubernetes enrichment), cold-page histograms through
`pkg/damon`, and, where present, GPU memory via `nvidia-smi` and vLLM
inference metrics. It spools locally with zstd rotation and can ship in
bundle mode. Systemd units and an install script are included.

Subcommands: `run` (default), `selftest` (capability matrix for a host),
`vllm-dump` (dumps every metric a vLLM endpoint exposes, handy for mapping a
new vLLM version), and `version`.

    curl -fsSL https://raw.githubusercontent.com/memaudit/memaudit/main/deploy/install.sh | sudo bash

Installs the latest release (`--version vX.Y.Z` to pin one), verifies its
checksum, and starts the service in sampling mode (`--mode zerotouch` for
the read-only-sysfs unit). See `deploy/install.sh` and `deploy/README.md`.

## Building

Needs Go 1.26+ and Linux. The agent reads procfs/sysfs/cgroupfs directly, so
it won't run (or build much of anything useful) on other platforms.

    git clone https://github.com/memaudit/memaudit.git
    cd memaudit
    go build ./cmd/memauditd

## Usage

    ./memauditd selftest                                 # capability matrix for this host
    ./memauditd run --config /etc/memaudit/config.yaml   # run the agent (default subcommand)
    ./memauditd vllm-dump --endpoint http://127.0.0.1:8000

Run `selftest` before deploying anywhere new. It checks which collectors a
host actually supports (DAMON's kernel rung, whether PSI is compiled in, and
so on) without collecting anything itself. See `deploy/` for the systemd
units and `install.sh`.

## memaudit-synth: the correctness proof

`memaudit-synth` is the standalone tool this project's own claims are
checked against: it mmaps a known amount of memory, keeps a known subset of
it actively touched, and leaves the rest idle, then checks that
`memauditd`'s DAMON-based cold-page histogram reports the idle portion
correctly. If DAMON can't see synthetic cold memory accurately, nothing
downstream is trustworthy. Run this yourself before trusting anything else
in this repo.

    go build ./cmd/memaudit-synth
    sudo ./memaudit-synth --alloc 8GiB --hot 2GiB --duration 10m   # needs root

Also ships as a prebuilt `memaudit-synth` binary in each release archive
alongside `memauditd`, if you'd rather not build it yourself. `--duration`
defaults to 10 minutes and has a 6-minute floor (`cold_300s` only becomes
meaningful once a region has been idle for 300s); `--version` prints the
build version.

Needs root (same as `memauditd`'s DAMON collector) and a box that isn't also
running `memauditd` at the same time: DAMON only has one monitoring slot,
and `memaudit-synth` checks for and refuses to steal a slot already in use
unless `--force` is passed. It also expects a box with enough headroom over
`--alloc` for the OS to stay healthy, but not so much spare RAM that the
run stops being a meaningful test. DAMON watches the whole machine's
physical memory, not just this tool's own allocation, so a much bigger or
busier box will show more "cold" memory than just what this tool itself
left idle. `memaudit-synth` checks both up front and aborts with an
explanation if the box looks unsuitable (`--force` to override).

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Commits need
a DCO sign-off (`git commit -s`), and PR titles follow [Conventional
Commits](https://www.conventionalcommits.org/).

## Security

`memauditd` typically runs as root and reads procfs/sysfs/cgroupfs, so
please report vulnerabilities privately instead of opening a public issue.
See [SECURITY.md](SECURITY.md) for how.

## License

Apache-2.0. See [LICENSE](LICENSE).
