#!/bin/sh
# Installs neovim from the official PPA
# (https://neovim.io/doc/install/#ubuntu). Runs as root on every boot, so it
# must be idempotent: add-apt-repository is a no-op once the repo is present.
set -eux

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y software-properties-common
add-apt-repository -y ppa:neovim-ppa/stable

apt-get update
apt-get install -y neovim
