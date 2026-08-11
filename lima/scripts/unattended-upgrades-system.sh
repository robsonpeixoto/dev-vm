#!/bin/sh
# Turns on unattended security upgrades for the whole OS. Runs as root on
# every boot, so it must be idempotent: the install is skipped once the
# package is there, and the systemd units tolerate being enabled twice.
#
# The policy itself lives in the two `mode: data` files applied before this
# script: /etc/apt/apt.conf.d/20auto-upgrades (run the periodic jobs at all)
# and 52dev-vm-unattended-upgrades (which origins, reboot policy).
set -eux

export DEBIAN_FRONTEND=noninteractive

if ! dpkg-query -W -f='${db:Status-Status}' unattended-upgrades 2>/dev/null |
    grep -qx installed; then
    apt-get update
    apt-get install -y unattended-upgrades
fi

# apt-daily fetches the lists, apt-daily-upgrade applies the upgrade; the
# service is what the timer starts. Ubuntu ships all three enabled, but the
# VM should not depend on that staying true.
systemctl enable --now apt-daily.timer apt-daily-upgrade.timer
systemctl enable unattended-upgrades.service

# The Docker updater in /usr/local/lib/dev-vm/cron.d/10-update-docker keeps
# running: Docker's apt repo has no security suite for the Allowed-Origins
# above to match. It already passes -o DPkg::Lock::Timeout=120, so it waits
# for an unattended-upgrades run instead of failing on the held lock.
