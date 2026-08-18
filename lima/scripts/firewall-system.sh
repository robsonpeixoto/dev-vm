#!/bin/sh
# Keeps the guest network wide open: the VM has its own vzNAT IP and no port
# forwards, so every port must be reachable at that IP. Runs as root on every
# boot, before any other system script, and is idempotent.
#
# Rootless Docker keeps its rules inside its own user network namespace, which
# nothing here touches; this script also runs before docker-user.sh sets the
# daemon up.
set -eux

export DEBIAN_FRONTEND=noninteractive

if ! dpkg-query -W -f='${db:Status-Status}' nftables 2>/dev/null | grep -qx installed; then
    apt-get update
    apt-get install -y nftables
fi

# Never `nft flush ruleset`: Lima owns `table ip nat`, whose LIMADNS chains
# redirect guest DNS from the prerouting and output hooks, and wiping it breaks
# name resolution until the next boot. Only packet-filter tables go, and
# deleting them (rather than flushing) drops their base chains, so a leftover
# `policy drop` cannot survive.
for family in inet ip ip6; do
    if nft list table "$family" filter >/dev/null 2>&1; then
        nft delete table "$family" filter
    fi
done

# ufw ships configured-inactive but with its unit enabled and running, so stop
# and mask it: no package upgrade or dotfiles checkout can switch a firewall
# back on.
if command -v ufw >/dev/null 2>&1; then
    ufw --force disable
fi
systemctl disable --now ufw.service || true
systemctl mask ufw.service

# Applies /etc/sysctl.d/99-dev-vm.conf, delivered as a mode: data file.
sysctl --system
