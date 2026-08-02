#!/bin/sh
# Installs zsh and makes it the guest user's login shell. Runs as root on
# every boot, so it must be idempotent. git is the oh-my-zsh installer's
# prerequisite (curl comes with docker-system.sh); the installer itself runs
# per-user in omz-user.sh.
set -eux

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y zsh git

chsh -s /usr/bin/zsh "$LIMA_CIDATA_USER"
