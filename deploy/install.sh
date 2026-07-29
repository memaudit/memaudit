#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 the memaudit authors
# SPDX-License-Identifier: Apache-2.0
#
# Installs memauditd from a GitHub release: detects arch, downloads the
# binary + checksums, verifies the checksum, installs both systemd unit
# files, writes a starter config if none exists, runs `memauditd
# selftest`, and enables+starts the service for the chosen mode.
#
# Usage: install.sh [--version vX.Y.Z] [--mode sampling|zerotouch] [--site NAME]
#
# Install locations (overridable for testing):
#   BIN_DIR, UNIT_DIR, CONFIG_DIR, STATE_DIR, DOWNLOAD_BASE, GITHUB_API
set -euo pipefail

REPO="memaudit/memaudit"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"
UNIT_DIR="${UNIT_DIR:-/etc/systemd/system}"
CONFIG_DIR="${CONFIG_DIR:-/etc/memaudit}"
STATE_DIR="${STATE_DIR:-/var/lib/memaudit}"
GITHUB_API="${GITHUB_API:-https://api.github.com}"
DOWNLOAD_BASE="${DOWNLOAD_BASE:-https://github.com/$REPO/releases/download}"

version=""
mode="sampling"
site=""

while [ $# -gt 0 ]; do
	case "$1" in
	--version)
		version="$2"
		shift 2
		;;
	--mode)
		mode="$2"
		shift 2
		;;
	--site)
		site="$2"
		shift 2
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
done

case "$mode" in
sampling | zerotouch) ;;
*)
	echo "--mode must be 'sampling' or 'zerotouch', got '$mode'" >&2
	exit 2
	;;
esac

os="$(uname -s)"
if [ "$os" != "Linux" ]; then
	echo "memauditd only runs on Linux, detected $os" >&2
	exit 1
fi

arch_raw="$(uname -m)"
case "$arch_raw" in
x86_64) arch="amd64" ;;
aarch64) arch="arm64" ;;
*)
	echo "unsupported architecture: $arch_raw (memauditd ships linux/amd64 and linux/arm64)" >&2
	exit 1
	;;
esac

if [ -z "$version" ]; then
	echo "resolving latest release..."
	version="$(curl -fsSL "$GITHUB_API/repos/$REPO/releases/latest" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
	if [ -z "$version" ]; then
		echo "could not resolve latest release version" >&2
		exit 1
	fi
fi
echo "installing memauditd $version ($arch)"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

archive="memaudit_linux_${arch}.tar.gz"
base_url="$DOWNLOAD_BASE/$version"

curl -fsSL -o "$workdir/$archive" "$base_url/$archive"
curl -fsSL -o "$workdir/checksums.txt" "$base_url/checksums.txt"

echo "verifying checksum..."
(cd "$workdir" && grep " $archive\$" checksums.txt | sha256sum -c -)

tar -xzf "$workdir/$archive" -C "$workdir" memauditd

install -d -m 0755 "$BIN_DIR"
install -m 0755 "$workdir/memauditd" "$BIN_DIR/memauditd"
echo "installed $BIN_DIR/memauditd"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
install -d -m 0755 "$UNIT_DIR"
install -m 0644 "$script_dir/memauditd.service" "$UNIT_DIR/memauditd.service"
install -m 0644 "$script_dir/memauditd-zerotouch.service" "$UNIT_DIR/memauditd-zerotouch.service"
echo "installed unit files to $UNIT_DIR"

install -d -m 0750 "$STATE_DIR/spool"

if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
	install -d -m 0755 "$CONFIG_DIR"
	site_value="${site:-CHANGE-ME}"
	cat >"$CONFIG_DIR/config.yaml" <<-EOF
		site: "$site_value"
		mode: $mode
		ship:
		  mode: bundle
	EOF
	echo "wrote starter config to $CONFIG_DIR/config.yaml — edit site/ship settings before relying on it"
else
	echo "$CONFIG_DIR/config.yaml already exists, leaving it as-is"
fi

echo
echo "running memauditd selftest..."
if ! "$BIN_DIR/memauditd" selftest; then
	echo
	echo "selftest failed — not enabling the service. Fix the host and re-run." >&2
	exit 1
fi

unit="memauditd.service"
if [ "$mode" = "zerotouch" ]; then
	unit="memauditd-zerotouch.service"
fi

systemctl daemon-reload
systemctl enable --now "$unit"
echo
echo "memauditd installed and running ($unit)."
