#!/bin/sh
# Populates ~/.ssh/known_hosts from the live github.com host keys instead of a
# pinned copy, so a GitHub key rotation does not break git over SSH. Runs as the
# guest login user (known_hosts is user state) on every boot, so it rewrites the
# file rather than appending duplicates. The retries cover transient DNS only —
# by user-mode provisioning the network is already up.
set -eux

mkdir -p ~/.ssh
chmod 700 ~/.ssh

for i in 1 2 3; do
	if ssh-keyscan -T 10 github.com >~/.ssh/known_hosts.new 2>/dev/null &&
		[ -s ~/.ssh/known_hosts.new ]; then
		mv ~/.ssh/known_hosts.new ~/.ssh/known_hosts
		chmod 644 ~/.ssh/known_hosts
		exit 0
	fi
	sleep "$i"
done

rm -f ~/.ssh/known_hosts.new
echo "ssh-keyscan github.com failed" >&2
exit 1
