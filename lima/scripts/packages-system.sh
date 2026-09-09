#!/bin/sh
# Installs the rest of the toolchain from the Ubuntu archive — everything that
# needs no third-party repo of its own. Runs as root on every boot, so it must
# be idempotent: once every package is installed the whole apt block is
# skipped; upgrading them is a manual apt-get upgrade.
#
# tig: terminal UI for git (git itself comes from git-system.sh).
# postgresql + libpq-dev: local database, plus the client headers that
#   database drivers link against.
# libnss3-tools: certutil, which mkcert and friends need to trust a local CA.
set -eux

export DEBIAN_FRONTEND=noninteractive

packages="tig postgresql libpq-dev libnss3-tools"

installed() {
    dpkg-query -W -f='${db:Status-Status}' "$1" 2>/dev/null | grep -qx installed
}

missing=0
for pkg in $packages; do
    installed "$pkg" || missing=1
done

if [ "$missing" = 1 ]; then
    apt-get update
    # shellcheck disable=SC2086
    apt-get install -y $packages
fi
