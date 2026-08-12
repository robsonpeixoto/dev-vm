#!/bin/sh
# Installs git from the official git-core PPA
# (https://launchpad.net/~git-core/+archive/ubuntu/ppa), which tracks upstream
# releases — Ubuntu's archive git only ever gets security fixes. Runs as root
# on every boot, so it must be idempotent: once the PPA and git are in place
# the whole block is skipped and upgrades are cron's job (11-update-git).
set -eux

export DEBIAN_FRONTEND=noninteractive

ppa_configured() {
    find /etc/apt/sources.list.d -name "git-core-ubuntu-ppa-*" | grep -q .
}

git_installed() {
    dpkg-query -W -f='${db:Status-Status}' git 2>/dev/null | grep -qx installed
}

if ! ppa_configured || ! git_installed; then
    # add-apt-repository refreshes the package lists itself, so no apt-get
    # update here.
    add-apt-repository -y ppa:git-core/ppa
    apt-get install -y git
fi
