#!/bin/sh
# Keeps the guest from upgrading itself. Ubuntu's cloud image has
# unattended-upgrades installed and apt-daily.timer / apt-daily-upgrade.timer
# enabled and active, so without this script the VM applies security patches
# on a randomized schedule of its own. Runs as root on every boot, so it must
# be idempotent: masking an already-masked unit is a no-op.
#
# The apt side of the switch is the `mode: data` file applied before this
# script, /etc/apt/apt.conf.d/99dev-vm-no-auto-upgrades (APT::Periodic all
# zero). Upgrading is manual:
#   limactl shell <name> sudo apt-get update && sudo apt-get upgrade
set -eux

# unattended-upgrades.service is the shutdown-time unit of the same package:
# it exists to finish an upgrade already in flight. Nothing here starts one, so
# it is masked too rather than left able to run apt at poweroff.
units="apt-daily.timer apt-daily-upgrade.timer
    apt-daily.service apt-daily-upgrade.service
    unattended-upgrades.service"

# Stop before masking, not `mask --now`: masking first leaves systemd unable to
# stop the running timer, which lands it in `failed (Result: resources)` and
# shows up in `systemctl --failed` for the life of the boot.
# shellcheck disable=SC2086
systemctl stop $units || true

# Masked, not just disabled: an apt upgrade can re-enable a disabled unit, and
# a masked unit cannot be started even while it is still enabled.
# shellcheck disable=SC2086
systemctl mask $units

# A unit that failed on an earlier boot (or that this script masked while it
# was running) stays failed until it is reset.
# shellcheck disable=SC2086
systemctl reset-failed $units 2>/dev/null || true
