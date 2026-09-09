#!/bin/sh
# Installs the Rust toolchain with rustup (https://rustup.rs), as the guest
# login user: rustup owns ~/.rustup and ~/.cargo, which makes the toolchain
# user state like the mise tools next to it, and lets `rustup update` and
# `cargo install` work without sudo.
#
# This is what neovim plugins with a Rust component build against — blink.cmp
# (https://github.com/saghen/blink.cmp) compiles its fuzzy matcher with
# `cargo build --release`, so it needs cargo and rustc, plus the git and C
# toolchain the system scripts already install. The distro cargo is not used:
# crates raise their minimum Rust version far faster than an LTS archive moves.
#
# Runs on every boot, so it must be idempotent: an existing toolchain is left
# alone, and upgrading it is the operator's `rustup update`.
set -eux

[ -x "$HOME/.cargo/bin/cargo" ] && exit 0

# --no-modify-path: PATH comes from /etc/profile.d/rust.sh, so rustup must not
# append its own line to ~/.zshrc or ~/.profile — the dotfiles own those.
# --profile minimal keeps it to cargo, rustc and rust-std; add clippy or
# rustfmt with `rustup component add` when they are wanted.
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs |
    sh -s -- -y --no-modify-path --profile minimal --default-toolchain stable
