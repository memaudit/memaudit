#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 the memaudit authors
# SPDX-License-Identifier: Apache-2.0
#
# Verifies memauditd is actually staying within its stated resource
# budget on this host: CPU, RSS, spool size, and parse errors. Reads
# systemd's own cgroup accounting rather than trusting the agent to
# report on itself — the whole point is an independent check.
#
# Usage: deploy/check-footprint.sh [--unit NAME] [--spool-dir PATH]
set -euo pipefail

unit="memauditd.service"
spool_dir="/var/lib/memaudit/spool"

while [ $# -gt 0 ]; do
	case "$1" in
	--unit)
		unit="$2"
		shift 2
		;;
	--spool-dir)
		spool_dir="$2"
		shift 2
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
done

# Budget, per the agent's own design target.
max_cpu_pct="0.5"
max_rss_bytes=$((64 * 1024 * 1024))
max_spool_bytes=$((2 * 1024 * 1024 * 1024))

show() {
	systemctl show "$unit" -p "$1" --value
}

start_ts="$(show ActiveEnterTimestamp)"
if [ -z "$start_ts" ] || [ "$start_ts" = "n/a" ]; then
	echo "unit $unit is not active — nothing to check" >&2
	exit 1
fi

start_epoch="$(date -d "$start_ts" +%s)"
now_epoch="$(date +%s)"
elapsed=$((now_epoch - start_epoch))
if [ "$elapsed" -lt 1 ]; then
	elapsed=1
fi

cpu_nsec="$(show CPUUsageNSec)"
rss_bytes="$(show MemoryCurrent)"

cpu_pct="$(awk -v ns="$cpu_nsec" -v secs="$elapsed" 'BEGIN { printf "%.3f", (ns / 1000000000) / secs * 100 }')"
cpu_ok="$(awk -v v="$cpu_pct" -v max="$max_cpu_pct" 'BEGIN { print (v < max) ? 1 : 0 }')"

rss_ok=0
[ "$rss_bytes" -lt "$max_rss_bytes" ] && rss_ok=1

spool_bytes="$(du -sb "$spool_dir" 2>/dev/null | cut -f1)"
spool_bytes="${spool_bytes:-0}"
spool_ok=0
[ "$spool_bytes" -lt "$max_spool_bytes" ] && spool_ok=1

parse_errors="$(journalctl -u "$unit" --since "$start_ts" -o cat 2>/dev/null | grep -c '"level":"ERROR"' || true)"
errors_ok=0
[ "$parse_errors" -eq 0 ] && errors_ok=1

to_mib() { awk -v b="$1" 'BEGIN { printf "%.1f", b / 1024 / 1024 }'; }

status() { [ "$1" -eq 1 ] && echo "OK" || echo "FAIL"; }

printf "CPU avg (%s%%, elapsed %ds, target <%s%%) ... %s\n" "$cpu_pct" "$elapsed" "$max_cpu_pct" "$(status "$cpu_ok")"
printf "RSS (%s MiB, target <64 MiB) ......... %s\n" "$(to_mib "$rss_bytes")" "$(status "$rss_ok")"
printf "spool (%s MiB, target <2048 MiB) ..... %s\n" "$(to_mib "$spool_bytes")" "$(status "$spool_ok")"
printf "parse errors (%s found, target 0) .... %s\n" "$parse_errors" "$(status "$errors_ok")"

if [ "$cpu_ok" -eq 1 ] && [ "$rss_ok" -eq 1 ] && [ "$spool_ok" -eq 1 ] && [ "$errors_ok" -eq 1 ]; then
	echo "verdict: within budget"
	exit 0
fi
echo "verdict: budget exceeded"
exit 1
