#!/bin/sh
# Installs the host mkcert root CA into the guest CAROOT, as the guest login
# user, so `mkcert <host>` in the VM issues certificates the host already
# trusts. devvm create stages the pair at /usr/local/lib/dev-vm from the host
# `mkcert -CAROOT` when settings.json has "mkcert": true; with the setting off
# both staged files are empty and this script does nothing.
#
# Trusting rootCA.pem in the guest system store is not this script's job: the
# template's caCerts.files entry hands it to cloud-init, which installs it
# under /usr/local/share/ca-certificates.
#
# Runs on every boot, so it must be idempotent: both files are rewritten.
set -eux

src=/usr/local/lib/dev-vm
[ -s "$src/rootCA.pem" ] || exit 0

# mkcert reads CAROOT first, then $XDG_DATA_HOME/mkcert, then
# ~/.local/share/mkcert. Provisioning has neither variable set, so the last one
# is the usual answer; ask the binary when it is on PATH already.
caroot=${CAROOT:-}
if [ -z "$caroot" ] && command -v mkcert >/dev/null 2>&1; then
    caroot=$(mkcert -CAROOT)
fi
[ -n "$caroot" ] || caroot="${XDG_DATA_HOME:-$HOME/.local/share}/mkcert"

install -d -m 755 "$caroot"
install -m 644 "$src/rootCA.pem" "$caroot/rootCA.pem"
if [ -s "$src/rootCA-key.pem" ]; then
    install -m 600 "$src/rootCA-key.pem" "$caroot/rootCA-key.pem"
fi
