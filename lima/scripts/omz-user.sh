#!/bin/sh
# User-mode oh-my-zsh provisioning: runs as the guest login user, after
# zsh-system.sh installed zsh and before dotfiles.sh — the installer writes a
# stock ~/.zshrc, and a dotfiles checkout replaces it with the real one.
# Runs on every boot; the installer refuses an existing ~/.oh-my-zsh, so skip.
set -eux

[ -d ~/.oh-my-zsh ] && exit 0

# CHSH=no: the login shell is already zsh (chsh done as root in
# zsh-system.sh; user chsh would prompt for a password). RUNZSH=no: do not
# exec an interactive zsh at the end.
RUNZSH=no CHSH=no sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended
