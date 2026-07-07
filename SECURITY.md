# Security policy

`memauditd` typically runs as root (see the systemd hardening in the build
docs) and reads procfs/sysfs/cgroupfs; a security issue here has real
blast radius on whatever host it's deployed on. Please report
responsibly rather than opening a public issue.

## Reporting a vulnerability

Preferred: use GitHub's private vulnerability reporting for this repo
("Security" tab → "Report a vulnerability").

Alternative: email info@hirt.cz with a description and, if possible, steps
to reproduce.

Please do not open a public GitHub issue for a suspected vulnerability
until it's been triaged.

## What to expect

This is a small project — best-effort response, but you should hear
something within 5 business days. If it's confirmed, we'll agree on a
disclosure timeline with you before any public writeup.

## Scope

In scope: `memauditd`, `pkg/damon`, `memaudit-synth`, and the deployment
artifacts in this repository.
