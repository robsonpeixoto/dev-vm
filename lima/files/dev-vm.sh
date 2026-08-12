# shellcheck shell=sh
# Marks the guest as a dev VM and names the Lima instance, so anything running
# inside (prompts, scripts, dotfiles) can branch on it.
#
# Installed globally, so no user dotfile is touched:
#   /etc/profile.d/dev-vm.sh  — bash login shells, and zsh login shells through
#                               /etc/zsh/zprofile -> /etc/profile
#   /etc/zsh/zshenv           — sources this file for every other zsh,
#                               including `limactl shell <name> <cmd>`
#
# {{.Name}} is the instance name, expanded on the host at creation time.
export DEV_VM=true
export DEV_VM_NAME='{{.Name}}'
:
