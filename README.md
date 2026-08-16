# memaudit

A read-only host agent for measuring cold (idle) memory, stranded DRAM, and
KV-cache waste on Linux hosts, using DAMON, cgroup v2, and PSI. No eBPF, no
kernel modules — everything here is procfs/sysfs/cgroupfs.

This repository contains the agent (`memauditd`), the DAMON client
(`pkg/damon`), the synthetic-workload correctness proof (`memaudit-synth`),
and deployment artifacts. The agent works standalone in bundle mode (local
JSONL, no server required).

**Status: core agent implemented, DAMON and GPU/vLLM collectors pending.**
`memauditd` collects host memory (meminfo, vmstat, PSI, NUMA, cgroup v2,
hugepages, optional Kubernetes enrichment), spools locally with zstd
rotation, and ships in bundle mode — systemd units and an install script are
included. `pkg/damon` (cold-page histograms) and the NVML/vLLM collectors
are not implemented yet. No versioned release has been cut yet, so
`install.sh` has nothing to fetch — build from source in the meantime.

## License

Apache-2.0. See [LICENSE](LICENSE). Contributions require a DCO sign-off —
see [CONTRIBUTING.md](CONTRIBUTING.md).
