Deployment artifacts land here: the sampling and zero-touch systemd
units, the DaemonSet manifest, and `install.sh`.

- `memauditd.service` / `memauditd-zerotouch.service` — the two systemd
  units from the security posture section. They differ by exactly one
  line (`ProtectKernelTunables=true`): the zero-touch unit makes `/sys`
  read-only, which also means DAMON (sampling mode's cold-page source)
  can't set itself up under that unit — pick zero-touch only when the
  customer's `config.yaml` also has `mode: zerotouch`.
- `install.sh` — fetches a release, verifies its checksum, installs the
  binary and both unit files, runs `memauditd selftest`, and
  enables+starts the service for the chosen mode.
- `daemonset.yaml` — not implemented yet.
