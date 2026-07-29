#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 the memaudit authors
# SPDX-License-Identifier: Apache-2.0
#
# Exercises install.sh end-to-end against a faked curl/systemctl and a
# throwaway root, without touching the network or the real service
# manager. Run via `task install-test` or directly: bash deploy/install_test.sh
#
# Requires Linux (install.sh itself refuses to run on anything else) —
# on macOS, run this inside a Linux container instead.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fail=0
case_status=0
case_work=""

# run_case builds a fresh fake root + fake PATH, runs install.sh with a
# fake memauditd binary whose `selftest` exits with $2, and leaves the
# outcome in $case_status / $case_work for the caller to assert on.
run_case() {
	local name="$1" selftest_exit="$2"
	local work fakebin archive
	work="$(mktemp -d)"
	fakebin="$work/fakebin"
	mkdir -p "$fakebin" "$work/root/bin" "$work/root/units" "$work/root/config" "$work/root/state" "$work/src"

	cat >"$work/src/memauditd" <<-EOF
		#!/usr/bin/env bash
		if [ "\${1:-}" = "selftest" ]; then
			echo "fake selftest output"
			exit $selftest_exit
		fi
		exit 0
	EOF
	chmod +x "$work/src/memauditd"

	archive="$work/memaudit_linux_amd64.tar.gz"
	(cd "$work/src" && tar -czf "$archive" memauditd)
	(cd "$work" && sha256sum "$(basename "$archive")" >checksums.txt)

	cat >"$fakebin/curl" <<-'EOF'
		#!/usr/bin/env bash
		out=""
		args=("$@")
		for i in "${!args[@]}"; do
			if [ "${args[$i]}" = "-o" ]; then
				out="${args[$((i + 1))]}"
			fi
		done
		url="${args[-1]}"
		case "$url" in
		*/releases/latest) echo '{"tag_name": "v0.1.0-test"}' ;;
		*checksums.txt) cp "$FAKE_CHECKSUMS" "$out" ;;
		*memaudit_linux_amd64.tar.gz) cp "$FAKE_ARCHIVE" "$out" ;;
		*)
			echo "fake curl: unhandled URL $url" >&2
			exit 1
			;;
		esac
	EOF
	chmod +x "$fakebin/curl"

	cat >"$fakebin/systemctl" <<-EOF
		#!/usr/bin/env bash
		echo "systemctl \$*" >>"$work/systemctl.log"
	EOF
	chmod +x "$fakebin/systemctl"

	local status=0
	PATH="$fakebin:$PATH" \
		FAKE_ARCHIVE="$archive" \
		FAKE_CHECKSUMS="$work/checksums.txt" \
		BIN_DIR="$work/root/bin" \
		UNIT_DIR="$work/root/units" \
		CONFIG_DIR="$work/root/config" \
		STATE_DIR="$work/root/state" \
		GITHUB_API="https://api.github.internal" \
		DOWNLOAD_BASE="https://dl.internal/releases" \
		"$root/deploy/install.sh" --site test-site >"$work/out.log" 2>&1 || status=$?

	echo "=== $name (install.sh exit $status) ==="
	cat "$work/out.log"

	case_status=$status
	case_work="$work"
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

# assert evaluates cond (a test(1) expression, e.g. "-f somefile") and
# reports it via check. Written as `cond_result=0; <test> || cond_result=1`
# rather than a bare `<test>; check ... $?` specifically because this
# script runs under `set -e`: a failing `[ ... ]` as the last command of
# a pipeline/statement would abort the whole script immediately, before
# `check` ever ran. Putting it on the left of `||` exempts it from that
# — only the *last* command in an OR-list can trigger errexit.
assert() {
	local desc="$1"
	shift
	local r=0
	"$@" || r=1
	check "$desc" "$r"
}

echo "--- case: selftest passes ---"
run_case "selftest-ok" 0
assert "install.sh exits 0" [ "$case_status" -eq 0 ]
assert "binary installed" [ -x "$case_work/root/bin/memauditd" ]
assert "sampling unit installed" [ -f "$case_work/root/units/memauditd.service" ]
assert "zero-touch unit installed" [ -f "$case_work/root/units/memauditd-zerotouch.service" ]
assert "config written with requested site" grep -q 'site: "test-site"' "$case_work/root/config/config.yaml"
assert "config defaults to bundle mode" grep -q 'mode: bundle' "$case_work/root/config/config.yaml"
assert "service enabled" grep -q 'enable --now memauditd.service' "$case_work/systemctl.log"

echo "--- case: selftest fails ---"
run_case "selftest-fails" 1
assert "install.sh exits non-zero" [ "$case_status" -ne 0 ]
assert "service NOT enabled" [ ! -f "$case_work/systemctl.log" ]

if [ "$fail" -ne 0 ]; then
	echo
	echo "install.sh test FAILED"
	exit 1
fi
echo
echo "install.sh test passed"
