#!/bin/sh
# Installs neovim from the official pre-built archives
# (https://neovim.io/doc/install/#pre-built-archives). The download and version
# check live in /usr/local/lib/dev-vm/install-neovim (a mode: data file) so this
# boot script and the 15-update-neovim cron job share one code path. Runs as
# root on every boot; the installer is idempotent.
set -eux

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y curl ca-certificates

/usr/local/lib/dev-vm/install-neovim
