#!/bin/sh
# Installs zsh and makes it the guest user's login shell. Runs as root on
# every boot, so it must be idempotent. git is the oh-my-zsh installer's
# prerequisite (curl comes with docker-system.sh); the installer itself runs
# per-user in omz-user.sh.
set -eux

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y zsh git

chsh -s /usr/bin/zsh "{{.User}}"

# zsh reads, in order: /etc/zsh/zshenv (always), ~/.zshenv, /etc/zsh/zprofile
# (login; it sources /etc/profile and therefore /etc/profile.d/*.sh), then
# ~/.zprofile, /etc/zsh/zshrc, ~/.zshrc. Only zshenv covers non-login
# non-interactive shells, which is what `limactl shell <name> <cmd>` and
# anything driving the VM over ssh get. Hooking DOCKER_HOST and DEV_VM there
# keeps them out of ~/.zshrc and ~/.bashrc, which a dotfiles checkout owns.
for f in docker-host.sh dev-vm.sh; do
    line="[ -r /etc/profile.d/$f ] && . /etc/profile.d/$f"
    grep -qxF "$line" /etc/zsh/zshenv 2>/dev/null || echo "$line" >>/etc/zsh/zshenv
done
