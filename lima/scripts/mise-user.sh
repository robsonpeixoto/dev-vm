#!/bin/sh
# User-mode mise provisioning: runs as the guest login user, after
# mise-system.sh installed the binary and after dotfiles.sh checked out any
# mise config the dotfiles carry. Trusts every config file and installs the
# tools they declare. Runs on every boot, so it must be idempotent (mise
# skips already-installed tools).
set -eux

# The oh-my-zsh mise plugin already runs `mise activate`; a second activation
# line on top of it would be redundant. Matches mise inside the plugins=(...)
# block, one-line or multiline.
omz_mise_enabled() {
	awk '
		/^[[:space:]]*plugins=\(/ { in_plugins = 1 }
		in_plugins && /(^|[( \t])mise([ \t)]|$)/ { found = 1 }
		in_plugins && /\)/ { in_plugins = 0 }
		END { exit !found }
	' ~/.zshrc 2>/dev/null
}

line='eval "$(mise activate zsh)"'
if ! omz_mise_enabled && ! grep -qxF "$line" ~/.zshrc 2>/dev/null; then
	echo "$line" >>~/.zshrc
fi

mise trust --all
mise install --yes
