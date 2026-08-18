#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 the memaudit authors
# SPDX-License-Identifier: Apache-2.0
#
# Exercises check-footprint.sh end-to-end against faked systemctl,
# journalctl, du, and cgroup memory.stat, without touching the real
# service manager. Run via `task check-footprint-test` or directly:
# bash deploy/check-footprint-test.sh
#
# Requires Linux (check-footprint.sh calls GNU date -d) — on macOS, run
# this inside a Linux container instead.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fail=0
case_status=0
case_out=""

# run_case builds a fake PATH (systemctl/journalctl/du) plus a fake
# cgroup tree with a memory.stat, then runs check-footprint.sh against
# it. Leaves the outcome in $case_status / $case_out for the caller to
# assert on. mem_anon is the "anon" field check-footprint.sh actually
# reads — a large file_cache value is included too, to prove the script
# ignores cache and doesn't false-fail on it.
run_case() {
	local active_enter="$1" cpu_nsec="$2" mem_anon="$3" spool_bytes="$4" journal_has_error="$5"
	local work fakebin
	work="$(mktemp -d)"
	fakebin="$work/fakebin"
	mkdir -p "$fakebin" "$work/spool" "$work/cgroup/system.slice/memauditd.service"

	cat >"$work/cgroup/system.slice/memauditd.service/memory.stat" <<-EOF
		anon $mem_anon
		file 999999999
		kernel 1000000
	EOF

	local journal_file="$work/journal.log"
	if [ "$journal_has_error" = "yes" ]; then
		echo '{"time":"2026-01-01T00:00:00Z","level":"ERROR","msg":"parse failed"}' >"$journal_file"
	else
		: >"$journal_file"
	fi

	cat >"$fakebin/systemctl" <<-'EOF'
		#!/usr/bin/env bash
		prop=""
		args=("$@")
		for i in "${!args[@]}"; do
			if [ "${args[$i]}" = "-p" ]; then
				prop="${args[$((i + 1))]}"
			fi
		done
		case "$prop" in
		ActiveEnterTimestamp) echo "$FAKE_ACTIVE_ENTER" ;;
		CPUUsageNSec) echo "$FAKE_CPU_NSEC" ;;
		ControlGroup) echo "/system.slice/memauditd.service" ;;
		esac
	EOF
	chmod +x "$fakebin/systemctl"

	cat >"$fakebin/journalctl" <<-EOF
		#!/usr/bin/env bash
		cat "$journal_file"
	EOF
	chmod +x "$fakebin/journalctl"

	cat >"$fakebin/du" <<-'EOF'
		#!/usr/bin/env bash
		printf '%s\t%s\n' "$FAKE_SPOOL_BYTES" "${*: -1}"
	EOF
	chmod +x "$fakebin/du"

	local status=0
	PATH="$fakebin:$PATH" \
		FAKE_ACTIVE_ENTER="$active_enter" \
		FAKE_CPU_NSEC="$cpu_nsec" \
		FAKE_SPOOL_BYTES="$spool_bytes" \
		"$root/deploy/check-footprint.sh" --spool-dir "$work/spool" --cgroup-root "$work/cgroup" >"$work/out.log" 2>&1 || status=$?

	case_status=$status
	case_out="$(cat "$work/out.log")"
}

check() {
	local desc="$1"
	if [ "$2" -eq 0 ]; then
		echo "ok - $desc"
	else
		echo "FAIL - $desc"
		fail=1
	fi
}

# assert evaluates cond and reports it via check. See install_test.sh
# for why this indirection matters under `set -e`.
assert() {
	local desc="$1"
	shift
	local r=0
	"$@" || r=1
	check "$desc" "$r"
}

assert_contains() {
	local desc="$1" needle="$2"
	local r=0
	echo "$case_out" | grep -qF "$needle" || r=1
	check "$desc" "$r"
}

now="$(date +%s)"
active_100s_ago="$(date -u -d "@$((now - 100))" +"%a %Y-%m-%d %H:%M:%S UTC")"

echo "--- case: within budget (large fake cache must not count against RSS) ---"
run_case "$active_100s_ago" 200000000 8388608 1048576 no
echo "$case_out"
assert "exits 0" [ "$case_status" -eq 0 ]
assert_contains "reports within budget" "verdict: within budget"
assert_contains "CPU line OK" "... OK"
assert_contains "RSS reports anon (8.0 MiB), not the ~954 MiB fake cache" "RSS anon (8.0 MiB"

echo "--- case: CPU over budget ---"
run_case "$active_100s_ago" 1000000000 8388608 1048576 no
echo "$case_out"
assert "exits non-zero" [ "$case_status" -ne 0 ]
assert_contains "reports budget exceeded" "verdict: budget exceeded"
assert_contains "CPU line FAILs" "FAIL"

echo "--- case: RSS over budget ---"
run_case "$active_100s_ago" 200000000 $((100 * 1024 * 1024)) 1048576 no
echo "$case_out"
assert "exits non-zero" [ "$case_status" -ne 0 ]
assert_contains "RSS line FAILs" "RSS"

echo "--- case: spool over budget ---"
run_case "$active_100s_ago" 200000000 8388608 $((3 * 1024 * 1024 * 1024)) no
echo "$case_out"
assert "exits non-zero" [ "$case_status" -ne 0 ]
assert_contains "spool line FAILs" "spool"

echo "--- case: parse errors present ---"
run_case "$active_100s_ago" 200000000 8388608 1048576 yes
echo "$case_out"
assert "exits non-zero" [ "$case_status" -ne 0 ]
assert_contains "parse errors line FAILs" "parse errors (1 found"

echo "--- case: unit not active ---"
run_case "" 0 0 0 no
echo "$case_out"
assert "exits non-zero" [ "$case_status" -ne 0 ]
assert_contains "reports not active" "is not active"

if [ "$fail" -ne 0 ]; then
	echo
	echo "check-footprint.sh test FAILED"
	exit 1
fi
echo
echo "check-footprint.sh test passed"
