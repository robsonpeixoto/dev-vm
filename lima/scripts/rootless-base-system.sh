#!/bin/sh
# Prepares the guest for rootless containers. Plain mode makes this script
# necessary: Lima's boot.sh runs boot.essential.Linux/* and then skips every
# boot.Linux/* script, so boot.Linux/20-rootless-base.sh — which normally does
# exactly this work — never executes. Without it
# dockerd-rootless-setuptool.sh fails with "could not find <user> in
# /etc/subuid", and even once that is fixed the daemon dies with the last
# session of the guest user. Runs as root on every boot, so it is idempotent.
set -eux

user="{{.User}}"
uid="{{.UID}}"

# newuidmap/newgidmap read these: rootlesskit needs a subordinate range to map
# the container's root onto. Same layout Lima uses — systemd-homed wants the
# range inside 524288-1878982656, and 1073741824 (1 G) is an arbitrary size
# leaving room above it for other accounts.
subid_begin=524288
[ "$uid" -ge "$subid_begin" ] && subid_begin=$((uid + 1))
for f in /etc/subuid /etc/subgid; do
    grep -qw "$user" "$f" 2>/dev/null || echo "$user:$subid_begin:1073741824" >>"$f"
done

# cgroup v2 delegation. Without a writable subtree under the user's own slice
# the rootless daemon cannot apply CPU or memory limits, so `docker run -m` and
# every compose file setting a limit fail.
install -d /etc/systemd/system/user@.service.d
cat >/etc/systemd/system/user@.service.d/dev-vm-delegate.conf <<'EOF'
[Service]
Delegate=yes
EOF
systemctl daemon-reload

# The rootless daemon is a systemd *user* unit, so it lives and dies with the
# user's systemd instance. linger keeps that instance running with nobody
# logged in, and it is also what creates /run/user/<uid> — which boot.sh waits
# for before running any `mode: user` script, and which docker-host.sh tests
# before exporting DOCKER_HOST.
systemctl start systemd-logind.service
loginctl enable-linger "$user"
