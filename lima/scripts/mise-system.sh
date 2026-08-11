#!/bin/sh
# Installs mise (https://mise.jdx.dev) from its official apt repo. Runs as
# root on every boot, so it must be idempotent — and behind packages_missing,
# so a boot with mise already installed downloads nothing. Tool trust/install
# happens per-user in mise-user.sh.
set -eux

# shellcheck source=../files/dev-vm-lib.sh disable=SC1091
. /usr/local/lib/dev-vm/lib.sh

packages_missing mise || exit 0

export DEBIAN_FRONTEND=noninteractive

install -dm 755 /etc/apt/keyrings
curl -fsSL https://mise.jdx.dev/gpg-key.pub | gpg --dearmor --yes -o /etc/apt/keyrings/mise-archive-keyring.gpg
chmod a+r /etc/apt/keyrings/mise-archive-keyring.gpg

architecture=$(dpkg --print-architecture)
echo "deb [signed-by=/etc/apt/keyrings/mise-archive-keyring.gpg arch=$architecture] https://mise.jdx.dev/deb stable main" \
	>/etc/apt/sources.list.d/mise.list

apt-get update
apt-get install -y mise
