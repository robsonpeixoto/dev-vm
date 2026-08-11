#!/bin/sh
# User-mode oh-my-zsh provisioning: runs as the guest login user, after
# zsh-system.sh installed zsh and before dotfiles.sh — the installer writes a
# stock ~/.zshrc, and a dotfiles checkout replaces it with the real one.
# Runs on every boot; the installer refuses an existing ~/.oh-my-zsh, so skip.
set -eux

[ -d ~/.oh-my-zsh ] && exit 0

# The installer is piped into sh, so it is remote code running as this user:
# pin it to a commit instead of following master, and check the download
# against the hash of that commit's file. Bumping it means changing both
# values. (The repository the installer then clones still tracks its default
# branch — omz updates itself in place, so pinning the checkout would only
# break `omz update`.)
omz_commit=b54a71977574cfcf659cc2f15a5e6422f17a8da7
omz_sha256=95118b50d062198597e2b73d3a57b609fd95ca68cdc86faf4460d955f0172b61
installer=$(mktemp)
trap 'rm -f "$installer"' EXIT

curl -fsSL "https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/$omz_commit/tools/install.sh" \
	-o "$installer"
echo "$omz_sha256  $installer" | sha256sum -c -

# CHSH=no: the login shell is already zsh (chsh done as root in
# zsh-system.sh; user chsh would prompt for a password). RUNZSH=no: do not
# exec an interactive zsh at the end.
RUNZSH=no CHSH=no sh "$installer" --unattended
