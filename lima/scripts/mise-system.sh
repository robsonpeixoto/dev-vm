#!/bin/sh
# Installs mise (https://mise.jdx.dev) from its official apt repo. Runs as
# root on every boot, so it must be idempotent: once mise is installed the
# whole block is skipped and upgrades are cron's job (12-update-mise). Tool
# trust/install happens per-user in mise-user.sh.
set -eux

export DEBIAN_FRONTEND=noninteractive

if ! dpkg-query -W -f='${db:Status-Status}' mise 2>/dev/null | grep -qx installed; then
    install -dm 755 /etc/apt/keyrings
    curl -fsSL https://mise.jdx.dev/gpg-key.pub | gpg --dearmor --yes -o /etc/apt/keyrings/mise-archive-keyring.gpg
    chmod a+r /etc/apt/keyrings/mise-archive-keyring.gpg

    architecture=$(dpkg --print-architecture)
    echo "deb [signed-by=/etc/apt/keyrings/mise-archive-keyring.gpg arch=$architecture] https://mise.jdx.dev/deb stable main" \
        >/etc/apt/sources.list.d/mise.list

    apt-get update
    apt-get install -y mise
fi
