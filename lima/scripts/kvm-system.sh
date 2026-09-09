#!/bin/sh
# Makes /dev/kvm usable without sudo, for a VM created with nested
# virtualization (devvm create -nested). Without nesting the host exposes no
# virtualization extensions, the guest kernel creates no /dev/kvm, and this
# script exits 0 having done nothing. Runs as root on every boot, so it is
# idempotent.
set -eux

[ -e /dev/kvm ] || exit 0

# udev gives /dev/kvm mode 0660 root:kvm, so membership is the whole story. The
# group is created only if the image lacks it; usermod -aG never drops the
# groups the user already has, and the new one applies to the next login — so
# the first `limactl shell` after provisioning already sees it.
getent group kvm >/dev/null || groupadd --system kvm
usermod -aG kvm "{{.User}}"
