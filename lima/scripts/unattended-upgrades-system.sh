#!/bin/sh
# Turns on unattended security upgrades for the whole OS. Runs as root on
# every boot, so it must be idempotent: the install is skipped once the
# package is there, and masking an already-masked unit is a no-op.
#
# The policy itself lives in the `mode: data` file applied before this script,
# /etc/apt/apt.conf.d/52dev-vm-unattended-upgrades (which origins, reboot
# policy).
set -eux

export DEBIAN_FRONTEND=noninteractive

if ! dpkg-query -W -f='${db:Status-Status}' unattended-upgrades 2>/dev/null |
    grep -qx installed; then
    apt-get update
    apt-get install -y unattended-upgrades
fi

# The packaged timers are a second apt scheduler: apt-daily fires anywhere in a
# 12-hour window, apt-daily-upgrade in a 1-hour one, and Persistent=true makes a
# missed run fire right after a VM start, next to boot provisioning. Masked, not
# just disabled, so a package upgrade cannot enable them again — the runner in
# /usr/local/sbin/dev-vm-cron calls unattended-upgrade itself from
# cron.d/05-upgrade-security, which keeps every apt run under one lock.
# A masked unit cannot be started even while it is still enabled, so mask
# --now both stops the timers and outranks any later enable.
systemctl mask --now apt-daily.timer apt-daily-upgrade.timer

# Not the timers' service: this one runs at shutdown to finish an upgrade that
# is already in flight, so it never starts one behind the runner's back.
systemctl enable unattended-upgrades.service
