# LIMA VM

This file is a reference for Lima v2.2.0
(https://github.com/lima-vm/lima/tree/v2.2.0). This repo targets that version
(`minimumLimaVersion: "2.0.0"` in `lima/dev-vm.yaml`).

## Summary: how Lima works

Lima runs a Linux guest from a YAML template. On macOS, the VM type is `vz`
(Apple Virtualization.framework) or `qemu`.

- `limactl start [--name <name>] <template.yaml>` creates a VM directory at
  `~/.lima/<name>/` and boots the VM. `--set '<yq expression>'` patches the
  template at creation time. `devvm create` uses it to set the
  `DOTFILES_REPO` param and the `.cpus`/`.memory`/`.disk` fields, all in one
  `|`-joined expression.
- At creation, Lima **flattens** the template. It merges the `base:`
  templates. It also puts the contents of the external `provision`/`probes`
  `file:` references into the stored `~/.lima/<name>/lima.yaml`. Later boots
  use that file as the source of truth, not the repo template. Thus an edit to
  `lima/dev-vm.yaml` or `lima/scripts/*.sh` has **no effect on an existing
  VM**. To apply the edit, recreate the VM
  (`go run . delete && go run . create`) or run `limactl edit <name>`.
- The VM directory holds `lima.yaml`, `basedisk`/`diffdisk`, `cidata.iso`,
  `ha.stdout.log`, `ha.stderr.log`, `serial*.log`.
- **cloud-init** delivers the guest configuration through `cidata.iso`. The
  guest mounts it read-only at `/mnt/lima-cidata` (mode 0700, root-only). The
  hostagent **regenerates `cidata.iso` from `lima.yaml` on every start**. As a
  result, a change that you make with `limactl edit` takes effect on the next
  boot.
- Networking: `vzNAT` gives the guest its own IP on a NAT interface. This repo
  uses `vzNAT` and disables all port forwards. Mounts exist only when the
  `mounts` setting asks for them (see [Host mounts](#host-mounts)). Without
  `networks:`, the default is user-mode networking plus host port forwarding.
- Other lifecycle commands: `limactl shell <name>`, `limactl stop <name>`,
  `limactl delete -f <name>`, `limactl list`, `limactl info`.

### Network invariants

These rules are not negotiable. They are the reason that `firewall-system.sh`
exists:

- Each VM has its own `vzNAT` IP, and that IP is the only way in. **Never add
  a `portForwards` entry.** The two `ignore: true` entries in
  `lima/dev-vm.yaml` cover `127.0.0.1` and `0.0.0.0`. These two entries must
  stay as they are.
- **Never install or enable a firewall in the guest.** `firewall-system.sh`
  runs first of the system scripts and enforces the opposite:
  - It deletes the `inet filter`, `ip filter` and `ip6 filter` tables. It
    deletes them and does not flush them. Thus the `policy drop` of a base
    chain cannot survive.
  - Then it runs `ufw --force disable`, `systemctl disable --now ufw.service`
    and `systemctl mask ufw.service`. The unit ships enabled *and* active,
    even while `ufw status` says inactive. Thus a mask alone leaves the unit
    in operation.
- **Never run `nft flush ruleset`.** Lima owns `table ip nat`. Its `LIMADNS`
  chains hook prerouting and output to redirect guest DNS. If you wipe the
  ruleset, name resolution breaks until the next boot. Rootless Docker keeps
  its rules in its own user network namespace. Those rules are out of reach
  either way.
- Every port must be usable, also the ports lower than 1024. For this, the
  guest needs `net.ipv4.ip_unprivileged_port_start=0`. The file
  `lima/files/sysctl/99-dev-vm.conf` holds the value, and `sysctl --system`
  in the same script applies it. Rootless Docker publishes ports as the guest
  user. Thus, with the 1024 default, `docker run -p 80:80` fails with
  `bind: permission denied`.

  By chance, Ubuntu 26.04 already ships 0. The file pins the value and does
  not trust that default.
- Lima does not report the guest IP:
  - `limactl list --format json` carries only `network[].macAddress`.
  - `sshAddress` is `127.0.0.1`.
  - macOS keeps stale rows in `/var/db/dhcpd_leases`. A test showed a lease
    row for `lima-default` with an IP from an earlier VM of that name. The
    same test showed no row for the current MAC.

  Thus `list`/`status`/`create` read the IP from the guest with
  `limactl shell <name> ip -4 -json addr show <iface>`. They take the
  interface name from `network[].interface`, which Lima itself reports. They
  make one attempt per Running VM, in parallel, with a 5 s deadline, through
  `limactlTry`. `limactlTry` is the only limactl helper that returns an error
  and does not call `die`. In all other cases, the output shows `-`.
- `limactl shell` and `~/.lima/<name>/ssh.config` still use the internal ssh
  tunnel of Lima on `127.0.0.1`. That tunnel is a Lima mechanism, not a port
  forward.

## Provisioning

`provision:` is a list of entries. Each entry has a `mode`. If an entry omits
`mode`, the mode is `system`.

### Cardinal rule: scripts run on EVERY boot

Lima connects provisioning to the `scripts_per_boot` of cloud-init
(`/var/lib/cloud/scripts/per-boot/00-lima.boot.sh`), not to `runcmd`. Every
`system`, `user`, `boot` and `dependency` script runs again on each restart.
Lima also applies every `data`/`yq` entry again.

**Scripts must be idempotent:**

- Guard each append (`grep -qxF ... || echo >>`).
- Use `install`/`mkdir -p`.
- Tolerate a state that is already done (`|| true`).
- If possible, rewrite a file. Do not append to it.

### Execution order (per boot)

1. cloud-init `init` stage, `bootcmd:`. The `mode: boot` scripts run here,
   very early. `/bin/sh` runs them directly, as root. They need no shebang.
   They get no `$PATH` conveniences and no network guarantees.
2. cloud-init `config` stage. cloud-init writes `00-lima.boot.sh`, but does
   not run it.
3. cloud-init `final` stage. `00-lima.boot.sh` runs
   `/mnt/lima-cidata/boot.sh`, which runs these steps in this order:
   - `boot.essential.Linux/*`, then `boot.Linux/00-…` … `25-…`.
   - `boot.Linux/30-install-packages.sh`. It runs the `mode: dependency`
     scripts first, then the dependency resolution of Lima.
   - `boot.Linux/35-…` and later (containerd, guestagent, and other
     scripts).
   - It copies the `mode: data` files.
   - It edits the `mode: yq` files.
   - It runs the `mode: system` scripts (as root).
   - It runs the `mode: user` scripts (as the guest login user).
   - It writes `/run/lima-boot-done`. Then the host stops its wait.

If a script fails, `boot.sh` collects a non-zero `CODE`, but it **continues**.
A failure of a provision script does not stop the boot. It only makes
`limactl start` report a provisioning failure.

### Plain mode skips every `boot.Linux/*` script

This repo uses `plain: true`. With it, `boot.sh` runs `boot.essential.Linux/*`
and then logs *"Plain mode. Skipping to run non-essential boot scripts"*. The
whole `boot.Linux/*` set never runs. The `data`/`yq`/`system`/`user`
provisioning and the probes still run. Thus this repo must supply everything
that the guest usually gets from those scripts:

- `boot.Linux/20-rootless-base.sh` supplies `/etc/subuid` + `/etc/subgid`
  ranges, `Delegate=yes` for `user@.service`, a started `systemd-logind`,
  `loginctl enable-linger`, and the `ip_unprivileged_port_start` /
  `ping_group_range` sysctls. Rootless Docker needs all of them:
  - Without the subid ranges, `dockerd-rootless-setuptool.sh` fails with
    `could not find <user> in /etc/subuid`.
  - Without linger, the systemd user instance that runs the daemon goes away
    with the last session. `/run/user/<uid>` goes away with it, and boot.sh
    waits for that directory before any `user` script.

  `scripts/rootless-base-system.sh` plus `files/sysctl/99-dev-vm.conf` replace
  this script.
- `boot.Linux/09-host-dns-setup.sh` makes the `LIMADNS` chains. This repo does
  not need them, because `vzNAT` gives DNS to the guest over DHCP.
- `boot.Linux/30-install-packages.sh` does not run, so the `mode: dependency`
  entries never run either. This repo has no such entries. Everything installs
  from `system` scripts.
- `boot.Linux/07-etc-environment.sh` does not run, so nothing writes the
  `env:` map of the template to `/etc/environment`. Ship env vars as
  `/etc/profile.d` files (`docker-host.sh`, `dev-vm.sh`) in its place.

### Modes

| Mode | Runs as | When | Payload field |
|---|---|---|---|
| `boot` | root, `/bin/sh` | cloud-init `bootcmd`, earliest | `script` |
| `dependency` | root | inside `30-install-packages.sh` | `script` |
| `data` | copied by root, chowned | after boot scripts, before system/user | `content` |
| `yq` | as `owner` | after `data` | `expression` |
| `system` | root | after data/yq | `script` |
| `user` | guest login user | last | `script` |
| `ansible` | — | deprecated, do not use | `playbook` |

Notes per mode:

- **`boot`**: runs before mounts and before packages. At that time, the
  network is not necessarily up. Use it only for things that must come before
  everything else. This repo uses no `boot` entry. A `boot` script runs as
  root before the guest login user exists. Thus a script that writes into
  `{{.Home}}` (`ssh-known-hosts.sh`, for example) puts its files in `/root`.

  For such scripts, use `user` mode.
- **`dependency`**: adds package repos or packages before Lima installs its
  own dependencies. To stop the default package installation of Lima
  completely, set `skipDefaultDependencyResolution: true` on at least one
  entry.
- **`data`**: writes a file and never runs it. It requires `path`. The content
  comes from `content:` or `file:`, but not from both. Defaults:
  `owner: "root:root"`, `permissions: 644`, `overwrite: true`. To write the
  file only if it is absent, set `overwrite: false`.

  Lima creates missing parent directories **as root**, whatever the `owner`
  value is. A test on Lima 2.2.0 showed this. A data file under
  `{{.Home}}/.config/...` left `~/.config` owned by root. As a result, the
  writes of the user in that directory failed. Thus never target a
  `{{.Home}}` subdirectory that does not already exist.

  Stage the file under `/usr/local/lib/dev-vm/` and copy it in a `user`
  script. At this point, the reverse-sshfs mounts are not up yet.
- **`yq`**: edits an existing file in place with a yq `expression`. If the
  file is missing, it creates the file. `format` defaults to `auto`, from the
  extension. For unrecognized extensions, set `format` explicitly. If `owner`
  cannot write the target, the entry fails.
- **`system`**: a root shell script. It needs a shebang. Use it for package
  installs, systemd unit management, and writes to `/etc` and `/usr/local`.
- **`user`**: runs through `sudo -iu <user>` with `XDG_RUNTIME_DIR` set. It
  runs after the systemd instance of the user is up (`boot.sh` waits for
  `/run/user/<uid>/systemd/private`). It needs a shebang. sudo keeps only the
  `PARAM_*` env vars. Use this mode for all work that touches the systemd
  session of the user, rootless daemons, or dotfiles.

### Inline script vs. `file:`

```yaml
- mode: system
  script: |
    #!/bin/sh
    set -eux
    ...
```

or

```yaml
- mode: system
  file:
    url: scripts/thing.sh   # relative to the template file
```

`file:` can be a plain string or an object with `url` and `digest`. Lima does
not use `digest` at this time. When `file` is set, `script`/`content` must be
empty. At VM creation, Lima reads the file and puts its contents into the
stored `lima.yaml` (see the flattening note above).

Prefer `file:` with a script in `lima/scripts/`. A script file is
shellcheck-able and diffable. Keep the shebang (`#!/bin/sh`) in the file.
Start the script with `set -eux`, so that failures show in the boot log.

### Template variables

Scripts, `data` `path`/`content`/`owner`, `yq` `expression`, and probes are Go
templates. The host evaluates them at creation time. The variables are
`{{.Home}}`, `{{.Name}}`, `{{.Hostname}}`, `{{.UID}}`, `{{.User}}`,
`{{.Param.Key}}`. `{{.Home}}` is the **guest** home (`/home/<user>.linux`).
Avoid a literal `{{`/`}}` that is for the guest, or escape it.

### Guest environment available to scripts

Before it runs anything, `boot.sh` exports `/mnt/lima-cidata/lima.env` and
`param.env`. Thus provision scripts see `LIMA_CIDATA_*`: `LIMA_CIDATA_USER`,
`LIMA_CIDATA_UID`, `LIMA_CIDATA_HOME`, `LIMA_CIDATA_NAME`, `LIMA_CIDATA_MNT`,
`LIMA_CIDATA_VMTYPE`, `LIMA_CIDATA_MOUNTS`, `LIMA_CIDATA_CONTAINERD_*`,
`LIMA_CIDATA_PLAIN`, `LIMA_CIDATA_PASSWORDLESS_SUDO`, plus `PARAM_*` from
`param:`. `user` mode also gets `XDG_RUNTIME_DIR=/run/user/<uid>`.

But these variables are internal. At creation time, the hostagent scans the
provision scripts and gives the warning `provisioning scripts should not
reference the LIMA_CIDATA variables`. Use the Go template variables above
(`{{.User}}`, `{{.Home}}`, `{{.UID}}`, `{{.Name}}`) in their place. `PARAM_*`
is still correct to use.

### Probes

`probes:` (`mode: readiness`) run as the user after provisioning.
`limactl start` does not complete before they pass. Each probe needs a `#!`
line. Add a `hint:`, which Lima shows on failure. If a later step (or the
operator) needs a service that is actually up, use a probe. Do not sleep
inside a provision script for this.

Only `script` is a Go template. `hint` is not, so write literal text in
`hint`.

This repo probes the two things that the VM exists for:

- The rootless Docker daemon answers `docker info`.
- `ssh -T git@github.com` reports `successfully authenticated`. That command
  exits non-zero even on success, so the probe matches on the output.

### Settings file

`~/.config/dev-vm/settings.json` is optional. It has exactly two root keys:

```json
{
  "default": {"cpus": 8, "memory": 16, "disk": 100,
              "dotfiles": "git@github.com:user/dotfiles.git",
              "mkcert": true,
              "ghostty": true, "nested": false,
              "mounts": [{"source": "~/Code", "destination": "~/host-code",
                          "mode": "rw"}],
              "clone": [{"org": "gnosispay", "basedir": "${HOME}/Code/gnosispay",
                         "repositories": ["gp-v2"]}]},
  "vms": {"new-vm": {"cpus": 4, "memory": 4, "clone": [],
                     "mkcert": false, "ghostty": false, "nested": true}}
}
```

- `loadSettings(name)` (`devvm.go`) decodes the file and returns one
  `vmConfig`. To make it, `mergeConfig` puts `vms.<name>` over `default`.
  `cmdCreate` is the only caller of `loadSettings`. No other part of the tool
  depends on the settings shape.
- Every field of `vmConfig` is a **pointer**. With a pointer, the code can
  tell an absent key from an explicit `"clone": []` or `"dotfiles": ""`. Both
  explicit values must turn the default off. They must not fall through to
  the default. An override replaces the full key. The merge does not
  concatenate `clone` lists.
- The decoder runs with `DisallowUnknownFields`. Thus a typo makes the create
  fail, and the tool does not ignore the typo. An old flat settings file (VM
  keys at the root) fails with a hint that names `default` and `vms`. There is
  no compatibility path. You must rewrite such a file.
- To read the file, the tool needs the VM name, and the name comes from
  `parseArgs`. Thus `cmdCreate` resolves the settings *after* flag parsing.
  For how the flags still win, see [VM size](#vm-size).

### Rust toolchain

`scripts/rust-user.sh` installs rustup (stable, `--profile minimal`) as the
guest login user. Neovim plugins with a Rust component need it. For example,
[blink.cmp](https://github.com/saghen/blink.cmp) builds its fuzzy matcher with
`cargo build --release`.

- **User mode, not a system package.** rustup owns `~/.rustup` and
  `~/.cargo`. Thus `rustup update`, `rustup component add` and `cargo install`
  need no sudo. `neovim-system.sh` installed `cargo` with apt for this purpose
  before, but it does not do this now. Crate MSRVs move much faster than an
  LTS archive. Also, two toolchains on one `PATH` are worse than one.

  Do not add `cargo` back to that package list. `build-essential` in that list
  is still necessary, because cargo needs a linker.
- **`PATH` comes from `files/rust.sh`** (`mode: data` →
  `/etc/profile.d/rust.sh`). If `~/.cargo/bin` exists, that file prepends it.
  Thus rustup runs with `--no-modify-path`. If rustup appends to `~/.zshrc` or
  `~/.profile`, it fights the dotfiles that own those files. `zsh-system.sh`
  lists that file with `docker-host.sh` and `dev-vm.sh` in the
  `/etc/zsh/zshenv` loop. That loop puts cargo in reach of a non-login shell,
  and of nvim that starts from such a shell.
- The script is idempotent because it skips the work. If `~/.cargo/bin/cargo`
  exists, the script stops. Thus a reboot never downloads the toolchain again
  and never upgrades it silently.
- The other build inputs come from other places and must stay:
  - `git` comes from `git-system.sh`. The `build.rs` of blink.cmp runs
    `git rev-parse HEAD`.
  - The linker comes from `build-essential`.

  blink.cmp needs Neovim 0.12+. The release tarball that `install-neovim`
  downloads already satisfies this.

### Dotfiles

`go run . create -dotfiles REPO` sets the `DOTFILES_REPO` param (through
`--set`). In the guest, `lima/scripts/dotfiles.sh` reads it as
`PARAM_DOTFILES_REPO`. The script clones the bare repo to `~/.dotfiles` and
checks it out over `$HOME`. If the param is empty, the script exits 0 and does
nothing.

- The repo can also come from the `dotfiles` key in
  `~/.config/dev-vm/settings.json`. That key turns dotfiles on for every VM.
  `-dotfiles REPO` overrides it, and `-no-dotfiles` skips it. A per-VM
  `"dotfiles": ""` turns dotfiles off again for that VM only. See [Settings
  file](#settings-file).
- Pre-existing files that the checkout overwrites move to
  `~/tmp/config-backup`. Each file keeps its relative path.
- A dotfiles checkout that owns `~/.zshrc`/`~/.bashrc` cannot break
  `DOCKER_HOST`. `/etc/profile.d/docker-host.sh` exports it globally. Login
  shells get that file through `/etc/profile` (zsh through
  `/etc/zsh/zprofile`). Every other zsh gets it through the line that
  `zsh-system.sh` adds to `/etc/zsh/zshenv`.
- **The dotfiles own `~/.ssh/config`. They must load every ssh config file
  that the VM ships.** Nothing in this repo writes that file. `mode: data`
  stages the GitHub identity (`lima/files/ssh-github.conf`) at
  `/usr/local/lib/dev-vm/ssh-github.conf`. Then `scripts/ssh-config-user.sh`
  installs it as `~/.ssh/config.d/10-github.conf`. Thus the ssh config of the
  dotfiles must contain `Include ~/.ssh/config.d/*.conf`.

  Put that line near the top, because ssh keeps the first value for each
  keyword. The `user` script creates the directory, not `mode: data`. Data
  mode creates missing parent directories as root. Then the user cannot put
  its own files in the directory.
- Without that `Include` line, or without dotfiles, the drop-in has no effect.
  The github probe still passes, because ssh tries `~/.ssh/id_ed25519` by
  default and `git@github.com` supplies the user. But `IdentitiesOnly yes` is
  not in effect.

### Repository clones

The `clone` key of the resolved settings block (see [Settings
file](#settings-file)) lists the repositories to clone in the guest. The list
groups them by GitHub org:

```json
{"default": {"clone": [{"org": "robsonpeixoto",
                        "basedir": "${HOME}/Code/robsonpeixoto",
                        "repositories": ["dev-vm", "echo-server"]}]}}
```

- `settingsClones` (`create.go`) validates the list and flattens it to one
  `{basedir, org/repo}` entry per repository. `cloneList` builds the guest
  list: tab-separated, one line per repository, after a comment header. The
  header is necessary, not decoration. When the settings list no clones, the
  header keeps the file non-empty. This is necessary because the `mode: data`
  entry for the file is unconditional.
- `startVM` writes the list to `tmp/clone-list` in the materialized template
  tree. The private key at `tmp/default` uses the same method. Then
  `mode: data` stages the list at `/usr/local/lib/dev-vm/clone-list`.
- `scripts/clone-user.sh` is the **last** `user` script. To clone, it needs
  the provisioned key, `known_hosts` and git from the earlier steps. The
  script does these steps:
  - It expands `${HOME}`/`$HOME` in `basedir` in the guest. The host cannot
    do this, because that path is the guest home.
  - If `<basedir>/<repo>` already exists, it skips that repository.
  - If a repository fails to clone, it logs the failure and skips that
    repository. Thus one unreachable repo does not stop the others.
- No flag configures clones. The setting is the only input. The tool reads it
  at create time, like everything else in the template. A per-VM
  `"clone": []` clones nothing for that VM. The default block stays intact
  for the other VMs.

### mkcert root CA

`"mkcert": true` in the resolved settings block copies the host mkcert CA into
the guest. As a result, both sides trust a certificate that either side
issues.

- `resolveCAROOT` (`create.go`) runs `mkcert -CAROOT` on the host. Both
  `rootCA.pem` and `rootCA-key.pem` must be in that directory. The setting
  asks for the CA. Thus a missing binary or a missing file is a `die`, not a
  silent skip. The tool copies the key with the certificate on purpose.
  Without the key, the guest can trust the CA but cannot issue certificates
  from it.
- `startVM` writes both files into `tmp/` of the materialized template tree.
  It uses the same method as for the private key and the clone list. Two
  unconditional `mode: data` entries stage the files at
  `/usr/local/lib/dev-vm/`. If the setting is off, the tool writes the files
  empty, and `mkcert-user.sh` then does nothing.

  `rootCA-key.pem` is staged with `owner: "{{.User}}"` and
  `permissions: 600`. The user script that installs the key cannot read a
  root-only file.
- `scripts/mkcert-user.sh` runs after `mise-user.sh`. mkcert usually comes
  from mise, so at that time `mkcert -CAROOT` can give the answer itself. The
  script installs the pair into `$CAROOT`, else into
  `${XDG_DATA_HOME:-$HOME/.local/share}/mkcert`. It must not write into
  `$HOME` through `mode: data`. `~/.local/share` also holds the tree of mise.
  If data mode creates that directory, it makes it owned by root.
- `startSet` adds `.caCerts.files = ["<host CAROOT>/rootCA.pem"]`. cloud-init
  installs that file into `/usr/local/share/ca-certificates` and trusts it
  with `update-ca-certificates`. That value is a **host** path in the
  `lima.yaml` of the VM. The hostagent reads it again on every start, when it
  regenerates `cidata.iso`. Thus the copy in the temp tree is not sufficient.

  If you delete the host CAROOT later, `devvm start` breaks for that VM.
- Nothing runs `mkcert -install` in the guest. `caCerts` covers the system
  store. The browser NSS store is the job of the operator
  (`packages-system.sh` installs `libnss3-tools`).

### ghostty terminfo

`"ghostty": true` compiles the `xterm-ghostty` terminfo entry in the guest.
Thus the `TERM` that Ghostty exports survives `limactl shell` and ssh.

- `hostTerminfo` (`create.go`) runs
  `$(brew --prefix ncurses)/bin/infocmp -x xterm-ghostty`. **It uses the
  Homebrew infocmp, not the macOS one.** `/usr/bin/infocmp` is ncurses
  6.0.20150808. It mangles the extended (`-x`) capabilities, and these
  capabilities are most of the entry. If the binary is missing, the result is
  a `die` that names `brew install ncurses`.
- The tool looks up the entry through `TERMINFO`. Ghostty sets `TERMINFO` to
  its app bundle. `create` can run outside Ghostty. Thus, if the dump fails,
  the tool tries again with
  `TERMINFO=/Applications/Ghostty.app/Contents/Resources/terminfo`.
- The tool stages the dump **base64-encoded**, through `terminfoB64`. This is
  not cosmetic. Lima runs `mode: data` content *and* provision scripts through
  a Go template on the host. The `acsc` capability of the entry lists the ACS
  pairs for the box-drawing characters, and doubled braces are among them. The
  template parser reads those braces as an action and rejects the file
  (`unexpected "|" in command`). Lima logs
  `Couldn't process data content as a template`.

  The payload survives only because Lima falls back to the unparsed text. The
  same trap caught a *comment* in `ghostty-terminfo-system.sh` that quoted the
  braces literally. That script failed to parse with
  `unterminated raw quoted string`. Never put doubled braces in a provision
  script. This rule includes comments.
- `scripts/ghostty-terminfo-system.sh` is a **system** script. It runs after
  `packages-system.sh` and does `base64 -d | tic -x -o /usr/share/terminfo -`.
  Thus root and sudo shells resolve the entry too, not only the login user. If
  `tic` is missing, the script installs `ncurses-bin`. If the staged file is
  empty, the script skips it.
- Nothing sets `TERM` in the guest. ssh and `limactl shell` carry the host
  value.

### Nested virtualization

`-nested` (or `"nested": true` in the resolved settings block) patches
`.nestedVirtualization = true` into the template. As a result, the guest
kernel gets virtualization extensions and creates `/dev/kvm`.

- `resolveNested` (`create.go`) is the boolean twin of `resolveResources`.
  When `fs.Visit` saw the flag, the flag wins **in both directions**. Thus
  `-nested=false` overrides a `default` block that turns nesting on. Only an
  absent flag falls through to the setting. There is no `-no-nested`, because
  a bool flag does not need one.
- `checkNested` runs before the tool registers the GitHub key. The reason: on
  unsupported hardware, vz fails the *start*, long after `create` changed
  state on GitHub. `checkNested` parses `machdep.cpu.brand_string` with
  `appleChipRE`. It requires `Apple M<n>` with `n >= 3` (`nestedMinChip`).
  Apple exposes no sysctl for the capability, and the probe in vz itself needs
  a running VM. The check rejects Intel and M1/M2.
- `startSet` writes the field **unconditionally** (`= true` or `= false`),
  unlike `caCerts.files`. Thus `nestedVirtualization: false` in
  `lima/dev-vm.yaml` is documentation, never the effective value.
- `scripts/kvm-system.sh` puts the login user in the `kvm` group. udev gives
  `/dev/kvm` mode 0660 `root:kvm`, and the Ubuntu image does not put the user
  in that group. Without this script, the device needs sudo. If `/dev/kvm` is
  absent, the script exits 0. That is the case for every VM that the tool
  creates without nesting.
- Nothing installs QEMU or Lima in the guest. That stays the job of the
  operator, like every other upgrade in this repo.

### Host mounts

The `mounts` key of the resolved settings block lists host directories to
share into the guest. Each entry is `{"source", "destination", "mode"}`.

- `settingsMounts` (`create.go`) validates the list and returns `[]mount` in
  Lima terms (`location`, `mountPoint`, `writable`). `source` and
  `destination` must be absolute, `~` or `~/...`. They must also match
  `mountPathRE` (`devvm.go`): no quotes, backslashes, braces or control
  characters. The reason: the paths go into a quoted yq string and then
  through the Go templates of Lima. `mode` is `"ro"` or `"rw"`. The default is
  `"ro"`, like `writable: false` in Lima.
- **`~` means a different home on each side.** On the host, Lima expands the
  tilde in `location`. But Lima rejects a `mountPoint` that starts with `~`
  (`validate.go`: "there is no tilde-expansion for guest filenames"). Thus the
  tool rewrites a `~` destination to `{{.Home}}...`, and Lima templates that
  to the guest home.

  If the destination is absent, the tool leaves it out of the entry
  completely. Lima then mounts at the expanded host path. Never default the
  destination to `source` verbatim. With that default, a `~/x` source becomes
  a `~/x` mount point, and Lima rejects it.
- `startSet` writes `.mounts = [...]` **unconditionally**. If the settings
  have no mounts, the list is empty. Thus the `mounts: []` of the template is
  documentation, like the size fields. Mounts are fixed at create time with
  everything else.
- Mounts need `plain: false`, because plain mode turns mounts off. The
  template has `plain: false` at this time.
- No flag configures mounts. A per-VM `"mounts": []` turns the default list
  off for that VM.

### VM size

`cpus`, `memory` and `disk` are **top-level template fields**, not params.
Thus `devvm create` patches them with
`.cpus = N | .memory = "NGiB" | .disk = "NGiB"` and not with `.param.*`.
`resolveResources` (`create.go`) resolves the size in this order:

1. It starts from `defaultResources` (2 vCPUs, 2 GiB, 50 GiB).
2. It applies the `cpus`/`memory`/`disk` keys of the resolved settings block.
3. It applies each of `-cpus`/`-memory`/`-disk` that the user actually passed.

The values in `lima/dev-vm.yaml` are documentation only. `--set` always
overwrites them.

The tool registers the flags with a **zero** default. It applies the settings
after parsing, not before. The reason: the settings block depends on the VM
name, and the name exists only after `parseArgs` runs. `flagsSet` (`fs.Visit`)
tells an unset flag from `-cpus 0`. Thus the zero default never gets into the
template.

Flags and settings are integers in GiB. The `flag` package rejects
non-integers. `resolveResources` (settings) and `checkResources` (flags)
reject non-positive values. All these rejections occur before the VM starts.

Lima flattens the template at creation. Thus the size is fixed for the life
of the VM. To change the size, run `limactl edit`, or delete the VM and
create it again.

`devvm disk-resize [name] <GiB>` (`diskresize.go`) is the one exception for
the disk:

- It reads the current size from `limaInstances`. If the new size is smaller,
  it refuses it before it touches the VM. Lima grows a disk, but never shrinks
  it. It also calls `loadSettings` first. Thus a broken settings file fails
  before the stop.
- The command runs `limactl edit --tty=false --disk N`. If the VM is
  `Running`, the command stops the VM before the edit and starts it again
  after the edit. For any other status, the command leaves the VM as it was.
  Without `--tty=false`, `edit` offers to start the VM itself.
- `setSettingsDisk` writes `vms.<name>.disk` **before** the restart. At that
  point, the edit is already done. Thus a failed start (a probe) must not
  leave the settings behind the VM.
- `withVMDisk` round-trips the file through `encoding/json/v2` with
  `jsontext.Value` leaves, not through `vmConfig`. Thus untouched blocks keep
  their key order (the tool redoes only the indentation), and nothing is lost.
  The tool marshals again only the root, `vms` and the edited block.
  `json.Deterministic` sorts them.

`go run . list` reads the live `cpus`/`memory`/`disk` from
`limactl list --format json`. In that output, memory and disk are in
**bytes**. These values are the declared *size* only. Lima reports no usage
figure. Thus disk pressure is a guest question (`docker system df`,
`df -h /`), and `list` deliberately stays out of it.

Nothing prunes the disk on a schedule, because the VM runs no cron at all. To
reclaim space, run `docker system prune` manually. See the disk hygiene
section of README.md.

### Guest OS pin

`base:` in `lima/dev-vm.yaml` names one release:
`template:_images/ubuntu-26.04`. It must never name
`template:_images/ubuntu-lts`. `ubuntu-lts.yaml` is a **symlink that ships
inside the installed Lima** (in v2.2.0, it points at `ubuntu-26.04.yaml`).
Thus it resolves against the Lima that is on the host. With `ubuntu-lts`, a
Lima upgrade moves new VMs to the next LTS with no change in this repo.

- Make sure that a template name exists. You do not need to boot anything for
  this. The name must exist as `_images/ubuntu-XX.YY.yaml` in
  `$(brew --prefix lima)/share/lima/templates/`, or under `templates/_images`
  at the matching upstream tag.
- Lima merges `base` at creation. Thus a change to the pin affects only the
  VMs that you create after the change. Existing VMs keep their release for
  life.
- The "Guest OS" section of README.md has the bump procedure and its new-LTS
  checklist (boot, docker probe, github probe). Bump the pin only after the
  apt repo of Docker carries the new codename. `docker-system.sh` gets the
  codename from the `VERSION_CODENAME` of the guest. Thus, if you bump first,
  provisioning breaks at `apt-get update`.

### Debugging

- Host: `~/.lima/<name>/ha.stderr.log`, `serial*.log`.
- Guest: `/var/log/cloud-init-output.log`. It holds the stdout/stderr of the
  provision scripts. The output of each script starts with
  `LIMA <timestamp>| Executing /mnt/lima-cidata/provision.<mode>/<NNNNNNNN>`.
- To run provisioning again without a new VM, run
  `limactl stop <name> && limactl start <name>`.
- To see what Lima actually shipped, run
  `limactl shell <name> sudo ls /mnt/lima-cidata` (root-only, mode 0700).
  Also read `~/.lima/<name>/lima.yaml` on the host.

### Writing provision scripts for this repo

- Put executable logic in `lima/scripts/*.sh`. Refer to it with `file.url`.
- Start each script with `#!/bin/sh` + `set -eux`. If the output can leak
  secrets, use `set -eu` in its place.
- Indent with 4 spaces, not tabs. Before you commit, run `make format`. Never
  run a bare `shfmt`/`gofmt`, so that the flags stay in one place.

  The `Makefile` holds `format`/`check-format` and their `-go`/`-shell`
  halves. `SHELL_DIRS` (`lima completions`) is the list that shfmt walks.
  shfmt finds shell files by extension or by shebang. Thus the extensionless
  `lima/files/install-neovim` needs no entry in that list.

  The width comes from the `.editorconfig` at the repo root and from its
  `[[shell]]` section. `root = true` makes that file cover every directory.
  The `[[shell]]` section is an shfmt extension that also matches files by
  shebang.

  In CI, the `check` job runs `make check-format-go`. The `shell` job runs
  `make check-format-shell` plus `shellcheck`, also with
  `-s bash completions/dev-vm.bash`. The zsh and fish completions are dialects
  that shellcheck cannot read.
- Make every script idempotent. The script runs again on every boot.
- Do root work in `system` scripts. Do per-user/systemd work in `user`
  scripts. Never mix the two.
- Never use `-o DPkg::Lock::Timeout`.

  When stdin is not a tty, apt already sets `Dpkg::Lock::Timeout` to 120 s by
  default (`BinarySpecificConfiguration` in `private-cmndline.cc` of apt,
  since apt 2.0). That default covers every non-interactive run. Also, the
  flag never applied to `apt-get update`. For that command, the lists lock
  goes through `pkgAcquire::GetLock` → `GetLock` (a non-blocking `F_SETLK`),
  not `GetLockMaybeWait`.

  The solution for contention is to have no second apt scheduler (see the
  no-auto-upgrade note below). The option is not the solution.
- Send secrets and configuration files through `mode: data` with an explicit
  `owner` and `permissions`. Do not write them with `echo` or a heredoc inside
  a script. Static payloads live in `lima/files/`. Refer to them with
  `file.url`, like the scripts.
- **Nothing in the guest upgrades itself, and nothing runs on a schedule.**
  There is no cron, no `cron.d`, and no `unattended-upgrades`. Boot
  provisioning installs what is missing and then stops. Upgrades are the job
  of the operator: `apt-get upgrade`, plus
  `/usr/local/lib/dev-vm/install-neovim` for neovim, which apt does not carry.
  Two parts enforce this rule, and both must stay:
  - `scripts/no-auto-upgrades-system.sh` **masks** `apt-daily.timer`,
    `apt-daily-upgrade.timer`, both of their services and
    `unattended-upgrades.service`. The timers fire in a random window. They
    are `Persistent=true`, so they catch up right after a VM start, at the
    same time as boot provisioning.
  - The `mode: data` file `lima/files/apt/99dev-vm-no-auto-upgrades` sets
    every `APT::Periodic` key to zero.

  Mask the units. Never only disable them. An apt upgrade enables a disabled
  unit again. Nothing can start a masked unit, even while it is still enabled.
- Do not add a scheduler again. A recurring job is a second apt client that
  races boot provisioning for the dpkg lock. It also causes version drift that
  nobody asked for. Anything periodic belongs on the host side of the VM, or in
  the hands of the operator.
- `//go:embed` (see `embed.go`) embeds the templates, scripts and static files
  into the Go binary. At create time, the tool materializes them into a temp
  dir. `go run .` gets edits automatically. You must rebuild a prebuilt
  `devvm` binary.
- If you change a template or a script, recreate the VM
  (`go run . delete <name> && go run . create <name>`). Otherwise, the change
  does not apply.
