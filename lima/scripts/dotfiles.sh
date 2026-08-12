#!/bin/sh
# Installs dotfiles from a bare repo checked out over $HOME (the
# --git-dir/--work-tree pattern), as the guest login user. No-op unless
# bin/create set the DOTFILES_REPO param. Runs on every boot, so it must be
# idempotent: re-checking out an already-checked-out tree changes nothing.
# Files that already exist and would be clobbered are moved under
# ~/tmp/config-backup keeping their relative path.
set -eu

repo=${PARAM_DOTFILES_REPO:-}
[ -n "$repo" ] || exit 0

set -x
cd "$HOME"

command -v git >/dev/null 2>&1 ||
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y git

config() {
    git --git-dir="$HOME/.dotfiles" --work-tree="$HOME" "$@"
}

[ -d "$HOME/.dotfiles" ] || git clone --bare "$repo" "$HOME/.dotfiles"

if ! config checkout; then
    backup="$HOME/tmp/config-backup"
    # `git checkout` lists the conflicting paths indented; move them aside so
    # the retry can win. The pipeline may match nothing, hence `|| true`.
    { config checkout 2>&1 || true; } | awk '/^\t|^ +/ {print $1}' |
        while read -r f; do
            [ -e "$HOME/$f" ] || continue
            mkdir -p "$backup/$(dirname "$f")"
            mv "$HOME/$f" "$backup/$f"
        done
    config checkout
fi

config config --local status.showUntrackedFiles no
config config --local branch.main.remote origin

# The checkout may have replaced ~/.ssh/config, which `mode: data` provisioned
# with the GitHub identity this VM's key depends on. ssh keeps the first value
# it sees for a keyword, so prepend that stanza to whatever the dotfiles ship.
stanza=$HOME/.ssh/lima-github.conf
conf=$HOME/.ssh/config
if [ -f "$stanza" ]; then
    marker=$(head -1 "$stanza")
    if ! grep -qxF "$marker" "$conf" 2>/dev/null; then
        touch "$conf"
        cat "$stanza" "$conf" >"$conf.new"
        mv "$conf.new" "$conf"
        chmod 600 "$conf"
    fi
fi

git --git-dir="${HOME}/.dotfiles/" --work-tree="${HOME}" branch --set-upstream-to=origin/main main
