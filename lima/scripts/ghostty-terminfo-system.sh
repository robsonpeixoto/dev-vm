#!/bin/sh
# Compiles the xterm-ghostty terminfo entry into the guest, as root, so
# TERM=xterm-ghostty works for every user — including root over sudo — and
# ncurses programs stop falling back to xterm-256color.
#
# devvm create captures the entry on the host with Homebrew's infocmp (the
# macOS built-in is too old for the extended capabilities the entry relies on)
# when settings.json has "ghostty": true, and stages it here base64-encoded —
# Lima runs `mode: data` content through a Go template, and the entry's acsc
# capability contains the doubled braces that parser reads as an action. With
# the setting off the staged file is empty and this script does nothing.
#
# Runs on every boot, so it must be idempotent: tic rewrites the entry.
set -eux

export DEBIAN_FRONTEND=noninteractive

src=/usr/local/lib/dev-vm/xterm-ghostty.terminfo.b64
[ -s "$src" ] || exit 0

# tic lives in ncurses-bin, which the Ubuntu cloud image already carries; the
# install is the fallback for an image that does not.
if ! command -v tic >/dev/null 2>&1; then
    apt-get update
    apt-get install -y ncurses-bin
fi

# -x keeps the extended capabilities; without it the entry loses most of what
# makes it a ghostty entry. tic reads the decoded source from stdin.
base64 -d <"$src" | tic -x -o /usr/share/terminfo -
