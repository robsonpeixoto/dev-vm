#!/bin/sh
# System-mode Docker provisioning: runs as root on every boot, so it must be
# idempotent. Installs Docker Engine from Docker's official apt repo
# (including docker-ce-rootless-extras), then disables and masks the
# system-wide daemon: this VM runs Docker *rootless*, owned by the guest login
# user (see docker-user.sh), which is what lets that user run `docker` with no
# sudo and no docker-group membership at all.
set -eux

export DEBIAN_FRONTEND=noninteractive

installed() {
    dpkg-query -W -f='${db:Status-Status}' "$1" 2>/dev/null | grep -qx installed
}

# ca-certificates + curl fetch the repo key; uidmap (newuidmap/newgidmap) and
# dbus-user-session are rootless Docker's own prerequisites; passt provides
# pasta, the network driver selected by the docker.service override file.
# docker-ce-rootless-extras carries dockerd-rootless-setuptool.sh, rootlesskit
# and slirp4netns — without it there is no rootless daemon to set up.
packages="ca-certificates curl uidmap dbus-user-session passt
    docker-ce docker-ce-cli containerd.io docker-ce-rootless-extras
    docker-buildx-plugin docker-compose-plugin"

# Only the first boot goes to the network; once everything is installed the
# whole apt block is skipped and upgrades are cron's job (10-update-docker).
missing=0
for pkg in $packages; do
    installed "$pkg" || missing=1
done

if [ "$missing" = 1 ]; then
    apt-get update
    apt-get install -y ca-certificates curl

    # Docker's official apt repo (see https://docs.docker.com/engine/install/ubuntu/).
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
    chmod a+r /etc/apt/keyrings/docker.asc

    architecture=$(dpkg --print-architecture)
    # shellcheck source=/dev/null
    codename=$(. /etc/os-release && echo "$VERSION_CODENAME")
    echo "deb [arch=$architecture signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $codename stable" \
        >/etc/apt/sources.list.d/docker.list

    apt-get update
    # shellcheck disable=SC2086
    apt-get install -y $packages
fi

# The rootless daemon runs its own dockerd + containerd inside the user's
# namespace, so the packaged system-wide units are not merely unused: a second
# daemon with a second image store is what makes `docker` ambiguous about which
# socket (and whose permissions) it is talking to. Mask them so nothing — an
# apt upgrade included — brings them back.
systemctl disable --now docker.service docker.socket containerd.service containerd.socket || true
systemctl mask docker.service docker.socket containerd.service containerd.socket || true

# The updater (/usr/local/lib/dev-vm/cron.d/10-update-docker), the cron runner
# and its /etc/cron.d entry ship as `mode: data` files, applied before this
# script runs. Package upgrades reach
# the user's running daemon on the next boot, when the rootless systemd user
# unit restarts.
