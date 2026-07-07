#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 the memaudit authors
# SPDX-License-Identifier: Apache-2.0
#
# Captures a real testdata/<name>/{proc,sys} fixture tree from the box this
# script runs on, for the collector golden tests in internal/collector.
# Run this on real target distro boxes (e.g. Ubuntu, Debian, a RHEL clone)
# to get real distro/kernel diversity — a container sharing your dev
# machine's kernel only proves the parsers, it isn't a substitute for a
# real capture.
#
# Usage: scripts/capture-fixtures.sh [name]
#   name defaults to the box's hostname.
set -euo pipefail

name="${1:-$(hostname -s)}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="$root/testdata/$name"

mkdir -p "$dest/proc/pressure" "$dest/sys/devices/system/node" "$dest/expected"

cp /proc/meminfo "$dest/proc/meminfo"
cp /proc/vmstat "$dest/proc/vmstat"
echo "captured proc/meminfo, proc/vmstat"

if [ -r /proc/pressure/memory ]; then
	cp /proc/pressure/memory "$dest/proc/pressure/memory"
	echo "captured proc/pressure/memory"
else
	echo "proc/pressure/memory not available (CONFIG_PSI off?) — skipped"
fi

node_root=/sys/devices/system/node
if [ -d "$node_root" ]; then
	for node in "$node_root"/node[0-9]*; do
		[ -d "$node" ] || continue
		n="$(basename "$node")"
		mkdir -p "$dest/sys/devices/system/node/$n"
		[ -r "$node/meminfo" ] && cp "$node/meminfo" "$dest/sys/devices/system/node/$n/meminfo"
		[ -r "$node/numastat" ] && cp "$node/numastat" "$dest/sys/devices/system/node/$n/numastat"
		echo "captured sys/devices/system/node/$n"
	done
else
	echo "$node_root not available (single-node host?) — skipped"
fi

cat <<EOF

Captured into $dest.

This only captures raw proc/sys input — it does not generate
testdata/$name/expected/*.json. Review the raw files, then populate the
expected/ JSON by running the collectors against this fixture directory
(see internal/collector/*_test.go for the pattern) and spot-checking the
output against the raw values before committing it as a golden fixture.
EOF
