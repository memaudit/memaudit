# memaudit

A read-only host agent for measuring cold (idle) memory, stranded DRAM, and
KV-cache waste on Linux hosts, using DAMON, cgroup v2, and PSI. No eBPF, no
kernel modules — everything here is procfs/sysfs/cgroupfs.

This repository contains the agent (`memauditd`), the DAMON client
(`pkg/damon`), the synthetic-workload correctness proof (`memaudit-synth`),
and deployment artifacts. The agent works standalone in bundle mode (local
JSONL, no server required).

**Status: early scaffold.** Collectors, spool, DAMON client, and deployment
units are not implemented yet.

## License

Apache-2.0. See [LICENSE](LICENSE). Contributions require a DCO sign-off —
see [CONTRIBUTING.md](CONTRIBUTING.md).
