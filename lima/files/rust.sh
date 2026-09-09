# shellcheck shell=sh
# Put the rustup shims on PATH. rust-user.sh installs rustup with
# --no-modify-path, so nothing appends a PATH line to a user dotfile: this file
# is the PATH entry, and ~/.cargo/bin holds both the shims (cargo, rustc,
# rustup) and whatever `cargo install` puts there.
#
# Installed globally, so no user dotfile is touched:
#   /etc/profile.d/rust.sh  — bash login shells, and zsh login shells through
#                             /etc/zsh/zprofile -> /etc/profile
#   /etc/zsh/zshenv         — sources this file for every other zsh, including
#                             `limactl shell <name> <cmd>` and any nvim started
#                             from one, which is what lets a plugin build shell
#                             out to cargo
#
# The directory test keeps PATH alone for users without a toolchain (root has
# no ~/.cargo), and the case guard keeps re-sourcing from duplicating the entry.
if [ -d "$HOME/.cargo/bin" ]; then
    case ":$PATH:" in
    *":$HOME/.cargo/bin:"*) ;;
    *) PATH="$HOME/.cargo/bin:$PATH" ;;
    esac
    export PATH
fi
:
