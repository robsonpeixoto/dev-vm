#!/bin/sh
# Installs neovim from the official pre-built archives
# (https://neovim.io/doc/install/#pre-built-archives). The download and version
# check live in /usr/local/lib/dev-vm/install-neovim (a mode: data file) so this
# boot script and the 15-update-neovim cron job share one code path. Runs as
# root on every boot, so it must be idempotent: once everything is installed
# both blocks are skipped and upgrades are that cron job's work — including the
# GitHub round-trip the installer makes to resolve the latest release.
#
# The plugin build toolchain comes from apt instead, and the tarball ships none
# of it: nvim-treesitter shells out to tree-sitter-cli and to a C compiler to
# build parsers, luarocks (with luajit) builds the Lua rocks plugins depend on,
# and cargo builds the Rust-based ones.
set -eux

export DEBIAN_FRONTEND=noninteractive

installed() {
    dpkg-query -W -f='${db:Status-Status}' "$1" 2>/dev/null | grep -qx installed
}

missing=0
for pkg in curl ca-certificates tree-sitter-cli build-essential luarocks luajit cargo; do
    installed "$pkg" || missing=1
done

if [ "$missing" = 1 ]; then
    apt-get update
    apt-get install -y curl ca-certificates tree-sitter-cli build-essential luarocks luajit cargo
fi

[ -x /usr/local/bin/nvim ] || /usr/local/lib/dev-vm/install-neovim
