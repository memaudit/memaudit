Golden fixtures used by the collector parser tests in `internal/collector`:
`proc`/`sys` input trees plus `expected/*.json` for the parsed output.

- `container-linux-6.12/` — meminfo, vmstat, and PSI are real captures
  (`docker run --rm ubuntu:24.04 cat /proc/meminfo /proc/vmstat
  /proc/pressure/memory`), kernel 6.12.76 aarch64 (Docker Desktop's
  linuxkit VM). **The NUMA sysfs files
  (`sys/devices/system/node/node{0,1}/*`) are hand-authored**, not a real
  capture — that VM is single-node and doesn't expose
  `/sys/devices/system/node` to containers at all. The format is stable
  and well-documented, but this should be replaced with a real capture
  from actual multi-socket hardware before relying on it for anything
  beyond exercising the parser.
- `edge-cases/` — deliberately synthetic, not meant to resemble a real
  box: `vmstat-old-kernel` drops the `workingset_refault_anon/file` split
  (added in kernel 5.8) to prove the vmstat parser tolerates missing keys;
  `psi-absent` has no `pressure/memory` file, to prove PSI-disabled hosts
  don't error. `vmstat-old-kernel/sys` also doesn't exist at all, which
  the NUMA, hugepages, and cgroup collector tests all reuse as their
  "no sysfs support for this at all" case — one absent directory serves
  three collectors' "absence is a valid state" tests, no need for three
  near-identical empty fixture trees. `cgroup-v1-host` has a
  controller-per-directory `sys/fs/cgroup` layout (`memory/`, `cpu/`)
  and, deliberately, no `cgroup.controllers` file at the root — that
  absence is what the cgroup collector actually keys off to detect v1
  and skip walking rather than misreading a v1 tree as v2.
- `hugepages-multi-node/` — hand-authored (not a real capture; see
  "Get more real fixtures" below), since neither of this repo's real
  capture sources exposes per-node hugepages sysfs. Exercises two page
  sizes: one broken out per NUMA node (and missing `resv_hugepages` on
  one node, to prove missing per-node files read as zero), one with no
  per-node breakout at all (falls back to the global host-level record,
  `node: -1`).
- `cgroup-v2-k8s/` — hand-authored synthetic `/sys/fs/cgroup` tree, not a
  real capture: real cgroup trees are host-specific and noisy, and this
  needs to hit a lot of specific shapes deliberately (a `system.slice`
  service, a full `kubepods.slice` → QoS slice → pod slice → container
  scope chain for k8s enrichment, an unrelated `user.slice` subtree that
  must never be walked, and files omitted here and there to prove
  missing `memory.{max,min,peak}` / `memory.pressure` / `memory.stat`
  all degrade to null/zero rather than erroring). `scripts/capture-fixtures.sh`
  intentionally doesn't attempt to capture real cgroup trees for the
  same reason.

Also worth noting from the real capture above: `DirectMap4k/2M/1G` are
absent from that container's `/proc/meminfo` entirely — those fields are
x86-only, arm64 kernels don't populate them. The meminfo collector already
handles this the same way it handles any other missing field (zero, not
an error); the `container-linux-6.12` fixture is incidentally also a test
of that path, since it's an arm64 capture.

**Get more real fixtures**: run `task fixtures -- <name>` (wraps
`scripts/capture-fixtures.sh`) on a real box — Ubuntu 24.04 HWE, Debian
12, and a RHEL-clone are good targets for real distro/kernel diversity.
It captures the raw `proc`/`sys` files into `testdata/<name>/`, including
`sys/kernel/mm/hugepages` and any per-node hugepages sysfs; it does
not generate `expected/*.json` — do that
by running the collectors against the new fixture directory (see
`internal/collector/*_test.go` for the pattern) and spot-checking the
output against the raw values before committing it as a golden fixture.
It does not capture `/sys/fs/cgroup` at all — see `cgroup-v2-k8s/` above
for why that fixture stays hand-authored.
Only real bare-metal/cloud boxes get real kernel-version diversity — every
container on the same Docker Desktop VM shares its kernel, so more docker
images wouldn't add anything the `container-linux-6.12` fixture doesn't
already cover.
