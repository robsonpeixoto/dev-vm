#!/bin/sh
# Installs go from the longsleep PPA
# https://go.dev/wiki/Ubuntu
set -eux

export DEBIAN_FRONTEND=noninteractive

ppa_configured() {
    find /etc/apt/sources.list.d -name "longsleep-ubuntu-golang-*" | grep -q .
}

go_installed() {
    dpkg-query -W -f='${db:Status-Status}' golang-go 2>/dev/null | grep -qx installed
}

if ! ppa_configured || ! go_installed; then
    # add-apt-repository refreshes the package lists itself, so no apt-get
    # update here.
    add-apt-repository -y ppa:longsleep/golang-backports
    apt-get install -y golang-go
fi
