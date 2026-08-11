#!/bin/sh
# User-mode Docker provisioning: runs as the guest login user, after
# docker-system.sh has installed the packages. This is where the permission
# problem is actually solved — the daemon is set up *rootless*, so it runs as
# this user, on this user's own socket under $XDG_RUNTIME_DIR, and `docker`
# needs neither sudo nor docker-group membership. Runs on every boot, so it
# must be idempotent (the setup tool itself is).
set -eux

# dbus has to be up before the setup tool can talk to this user's systemd
# instance; the tool then installs, enables and starts the `docker` user unit,
# which Lima's enable-linger keeps running across logout and reboots.
systemctl --user start dbus
dockerd-rootless-setuptool.sh install
docker context use rootless

# The context above is enough for the docker CLI, but not for libraries that
# read DOCKER_HOST directly; /etc/profile.d/docker-host.sh (a `mode: data`
# file, wired into zsh by zsh-system.sh) exports it for every shell.
