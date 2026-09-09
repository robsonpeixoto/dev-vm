#!/bin/sh
# Installs the GitHub identity as a ~/.ssh/config.d drop-in, as the guest login
# user. ~/.ssh/config itself is left alone: loading the drop-ins is the
# dotfiles' job (`Include ~/.ssh/config.d/*.conf`). Runs on every boot, so it
# rewrites the file rather than appending. The directory is created here, not by
# `mode: data`, which would create the missing parent as root and lock the user
# out of its own drop-in directory.
set -eux

mkdir -p ~/.ssh/config.d
chmod 700 ~/.ssh ~/.ssh/config.d
install -m 600 /usr/local/lib/dev-vm/ssh-github.conf ~/.ssh/config.d/10-github.conf
