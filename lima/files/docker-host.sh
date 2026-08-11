# shellcheck shell=sh
# Point DOCKER_HOST at this user's rootless daemon socket. The docker CLI finds
# it through the `rootless` context, but libraries that read DOCKER_HOST
# directly (testcontainers, the Docker SDKs) otherwise fall back to the rootful
# /var/run/docker.sock that this VM deliberately does not have.
#
# Installed globally, so no user dotfile is touched:
#   /etc/profile.d/docker-host.sh  — bash login shells, and zsh login shells
#                                    through /etc/zsh/zprofile -> /etc/profile
#   /etc/zsh/zshenv                — sources this file for every other zsh,
#                                    including `limactl shell <name> <cmd>`
#
# The socket path contains the UID, so it is resolved per session. The
# directory test keeps DOCKER_HOST unset for users without a rootless daemon
# (root has no /run/user/0).
__dev_vm_runtime_dir="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
[ -d "$__dev_vm_runtime_dir" ] && export DOCKER_HOST="unix://$__dev_vm_runtime_dir/docker.sock"
unset __dev_vm_runtime_dir
:
