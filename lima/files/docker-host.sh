# shellcheck shell=sh
# Point DOCKER_HOST at this user's rootless daemon socket. The docker CLI finds
# it through the `rootless` context, but libraries that read DOCKER_HOST
# directly (testcontainers, the Docker SDKs) otherwise fall back to the rootful
# /var/run/docker.sock that this VM deliberately does not have. Sourced from
# ~/.bashrc; the path contains this user's UID, so it is resolved per session.
[ -n "${XDG_RUNTIME_DIR:-}" ] && export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/docker.sock"
:

