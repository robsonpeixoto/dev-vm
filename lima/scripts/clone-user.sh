#!/bin/sh
# Clones the repositories from the "clone" entry of
# ~/.config/dev-vm/settings.json, as the guest login user. devvm create renders
# them into /usr/local/lib/dev-vm/clone-list, one "<basedir>\t<org>/<repo>"
# line per repository, comment lines aside. Last user script: it needs the ssh
# key, known_hosts and git from the earlier steps.
# Runs on every boot, so it must be idempotent: an existing checkout is
# skipped, and a repository that fails to clone leaves the rest to proceed.
set -eux

list=/usr/local/lib/dev-vm/clone-list
[ -f "$list" ] || exit 0

tab=$(printf '\t')
while IFS="$tab" read -r basedir repo; do
    case $basedir in
    '' | \#*) continue ;;
    esac
    [ -n "$repo" ] || continue

    # ${HOME} in the setting is the guest home, so it is expanded here rather
    # than by the host that wrote the list.
    dir=$(printf '%s' "$basedir" | sed -e "s|\${HOME}|$HOME|g" -e "s|\$HOME|$HOME|g")
    dir="$dir/${repo#*/}"
    if [ -e "$dir" ]; then
        continue
    fi

    mkdir -p "$(dirname "$dir")"
    if ! git clone "git@github.com:$repo.git" "$dir" </dev/null; then
        echo >&2 "clone: $repo failed, skipping"
    fi
done <"$list"
