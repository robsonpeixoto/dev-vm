# LIMA VM

Reference for Lima v2.2.0 (https://github.com/lima-vm/lima/tree/v2.2.0), the
version this repo targets (`minimumLimaVersion: "2.0.0"` in `lima/dev-vm.yaml`).

## Summary: how Lima works

Lima runs a Linux guest from a YAML template. On macOS the VM type is `vz`
(Apple Virtualization.framework) or `qemu`.

- `limactl start [--name <name>] <template.yaml>` creates an instance directory
  at `~/.lima/<name>/` and boots it. `--set '<yq expression>'` patches the
  template at creation time (used by `devvm create` to set the
  `DOTFILES_REPO` param and the `.cpus`/`.memory`/`.disk` fields, all in one
  `|`-joined expression).
- The template is **flattened at creation**: `base:` templates are merged, and
  external `provision`/`probes` `file:` references are inlined into the stored
  `~/.lima/<name>/lima.yaml`. That file — not the repo template — is the source
  of truth for later boots. Editing `lima/dev-vm.yaml` or `lima/scripts/*.sh`
  has **no effect on an existing VM**; recreate it
  (`go run . delete && go run . create`) or `limactl edit <name>`.
- Instance directory holds `lima.yaml`, `basedisk`/`diffdisk`, `cidata.iso`,
  `ha.stdout.log`, `ha.stderr.log`, `serial*.log`.
- Guest configuration is delivered by **cloud-init** through `cidata.iso`,
  mounted read-only in the guest at `/mnt/lima-cidata` (mode 0700, root-only).
  The hostagent **regenerates `cidata.iso` from `lima.yaml` on every start**, so
  config changes made with `limactl edit` take effect on the next boot.
- Networking: `vzNAT` gives the guest its own IP on a NAT interface (this repo
  uses it and disables all port forwards and mounts). Default (no `networks:`)
  is user-mode networking plus host port forwarding.
- Other lifecycle commands: `limactl shell <name>`, `limactl stop <name>`,
  `limactl delete -f <name>`, `limactl list`, `limactl info`.

### Network invariants

Non-negotiable, and the reason `firewall-system.sh` exists:

- Every VM has its own `vzNAT` IP, and that IP is the only way in. **Never add
  a `portForwards` entry** — the two `ignore: true` entries in
  `lima/dev-vm.yaml` cover `127.0.0.1` and `0.0.0.0` and stay that way.
- **Never install or enable a firewall in the guest.** `firewall-system.sh` runs
  first among the system scripts and enforces the opposite: it deletes the
  `inet filter`, `ip filter` and `ip6 filter` tables (delete, not flush, so a
  base chain's `policy drop` cannot survive), then `ufw --force disable`,
  `systemctl disable --now ufw.service` — the unit ships enabled *and* active
  even while `ufw status` says inactive, so masking alone would leave it
  running — and `systemctl mask ufw.service`.
- **Never `nft flush ruleset`.** Lima owns `table ip nat`, whose `LIMADNS`
  chains hook prerouting and output to redirect guest DNS; wiping the ruleset
  breaks name resolution until the next boot. Rootless Docker's rules live in
  its own user network namespace and are out of reach either way.
- Every port must be usable, including below 1024. That needs
  `net.ipv4.ip_unprivileged_port_start=0`
  (`lima/files/sysctl/99-dev-vm.conf`, applied by `sysctl --system` in the same
  script): rootless Docker publishes ports as the guest user, so the 1024
  default makes `docker run -p 80:80` fail with `bind: permission denied`.
  Ubuntu 26.04 happens to ship 0 already; the file pins it instead of trusting
  that default.
- Lima does not report the guest IP. `limactl list --format json` carries only
  `network[].macAddress`, `sshAddress` is `127.0.0.1`, and macOS's
  `/var/db/dhcpd_leases` keeps stale rows (verified: a lease row for
  `lima-default` holding an IP from an earlier incarnation of the instance, and
  no row for the current MAC). So `list`/`status`/`create` read it from the
  guest with `limactl shell <name> ip -4 -json addr show <iface>`, taking the
  interface name from Lima's own `network[].interface`. One attempt per Running
  VM, in parallel, 5 s deadline, through `limactlTry` — the only limactl helper
  that returns an error instead of calling `die`. Anything else renders `-`.
- `limactl shell` and `~/.lima/<name>/ssh.config` still ride Lima's internal ssh
  tunnel on `127.0.0.1`. That is Lima's own mechanism, not a port forward.

## Provisioning

`provision:` is a list of entries, each with a `mode`. Default mode when
omitted is `system`.

### Cardinal rule: scripts run on EVERY boot

Provisioning is wired into cloud-init's `scripts_per_boot`
(`/var/lib/cloud/scripts/per-boot/00-lima.boot.sh`), not `runcmd`. Every
`system`, `user`, `boot`, `dependency` script runs again on each restart, and
every `data`/`yq` entry is reapplied. **Scripts must be idempotent.** Guard
appends (`grep -qxF ... || echo >>`), use `install`/`mkdir -p`, tolerate
already-done state (`|| true`), rewrite files rather than appending.

### Execution order (per boot)

1. cloud-init `init` stage — `bootcmd:`: `mode: boot` scripts run here, very
   early, directly by `/bin/sh` (no shebang needed, no `$PATH` niceties, no
   network guarantees, run as root).
2. cloud-init `config` stage — `00-lima.boot.sh` is written, not run.
3. cloud-init `final` stage — `00-lima.boot.sh` runs `/mnt/lima-cidata/boot.sh`,
   which executes, in order:
   - `boot.essential.Linux/*` then `boot.Linux/00-…` … `25-…`
   - `boot.Linux/30-install-packages.sh` — runs `mode: dependency` scripts
     first, then Lima's own dependency resolution
   - `boot.Linux/35-…` and later (containerd, guestagent, etc.)
   - `mode: data` files are copied
   - `mode: yq` files are edited
   - `mode: system` scripts run (root)
   - `mode: user` scripts run (guest login user)
   - `/run/lima-boot-done` is written; the host stops waiting

`boot.sh` collects a non-zero `CODE` if any script fails but **keeps going** —
a failing provision script does not abort the boot, it just makes
`limactl start` report a provisioning failure.

### Plain mode skips every `boot.Linux/*` script

`plain: true` (this repo uses it) makes `boot.sh` run
`boot.essential.Linux/*` and then log *"Plain mode. Skipping to run
non-essential boot scripts"* — the whole `boot.Linux/*` set never executes,
while `data`/`yq`/`system`/`user` provisioning and the probes still do. So
anything the guest gets from those scripts is this repo's job:

- `boot.Linux/20-rootless-base.sh` — `/etc/subuid` + `/etc/subgid` ranges,
  `Delegate=yes` for `user@.service`, `systemd-logind` started,
  `loginctl enable-linger`, and the `ip_unprivileged_port_start` /
  `ping_group_range` sysctls. Rootless Docker needs all of it:
  `dockerd-rootless-setuptool.sh` fails with `could not find <user> in
  /etc/subuid` without the subid ranges, and without linger the daemon's
  systemd user instance (and `/run/user/<uid>`, which boot.sh waits for before
  any `user` script) goes away with the last session. Replaced by
  `scripts/rootless-base-system.sh` plus `files/sysctl/99-dev-vm.conf`.
- `boot.Linux/09-host-dns-setup.sh` — the `LIMADNS` chains. Not needed here:
  `vzNAT` hands the guest DNS over DHCP.
- `boot.Linux/30-install-packages.sh` — so `mode: dependency` entries never
  run either. This repo has none; everything installs from `system` scripts.
- `boot.Linux/07-etc-environment.sh` — the template's `env:` map is never
  written to `/etc/environment`. Ship env vars as `/etc/profile.d` files
  (`docker-host.sh`, `dev-vm.sh`) instead.

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

- **`boot`** — runs before mounts, before packages, before the network is
  necessarily up. Only for things that must precede everything else.
  This repo uses no `boot` entry: it runs as root before the guest login user
  exists, so anything writing into `{{.Home}}` (`ssh-known-hosts.sh`, for one)
  would land in `/root` instead. Use `user` mode for that.
- **`dependency`** — for adding package repos/packages before Lima installs
  its own dependencies. Set `skipDefaultDependencyResolution: true` on at
  least one entry to suppress Lima's default package installation entirely.
- **`data`** — writes a file, never executes it. Requires `path`; content
  comes from `content:` or `file:` (mutually exclusive). Defaults:
  `owner: "root:root"`, `permissions: 644`, `overwrite: true` (set
  `overwrite: false` to write only if absent). Missing parent directories are
  created **as root** regardless of `owner` (observed on Lima 2.2.0: a data
  file under `{{.Home}}/.config/...` left `~/.config` root-owned and broke the
  user's own writes there) — so never target a `{{.Home}}` subdirectory that
  does not already exist; stage the file under `/usr/local/lib/dev-vm/` and
  copy it in a `user` script instead. reverse-sshfs mounts are not up yet at
  this point.
- **`yq`** — edits an existing file in place with a yq `expression`, creating
  it if missing. `format` defaults to `auto` (from the extension); set it
  explicitly for unrecognized extensions. Fails if the target is not writable
  by `owner`.
- **`system`** — root shell script, needs a shebang. Package installs,
  systemd unit management, `/etc` and `/usr/local` writes.
- **`user`** — runs via `sudo -iu <user>` with `XDG_RUNTIME_DIR` set, after
  the user's systemd instance is up (`boot.sh` waits for
  `/run/user/<uid>/systemd/private`). Needs a shebang. Only `PARAM_*` env vars
  are preserved through sudo. Use this for anything touching the user's
  systemd session, rootless daemons, or dotfiles.

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

`file:` may be a plain string or an object with `url` and `digest` (`digest`
is currently unused). `script`/`content` must be empty when `file` is set. The
file is read and inlined at instance creation — see the flattening note above.

Prefer `file:` with a script in `lima/scripts/`: it is shellcheck-able and
diffable. Keep the shebang in the file (`#!/bin/sh`) and start with
`set -eux` so failures surface in the boot log.

### Template variables

Scripts, `data` `path`/`content`/`owner`, `yq` `expression`, and probes are Go
templates evaluated on the host at creation time:
`{{.Home}}`, `{{.Name}}`, `{{.Hostname}}`, `{{.UID}}`, `{{.User}}`,
`{{.Param.Key}}`. `{{.Home}}` is the **guest** home (`/home/<user>.linux`).
Literal `{{`/`}}` intended for the guest must be avoided or escaped.

### Guest environment available to scripts

`boot.sh` exports `/mnt/lima-cidata/lima.env` and `param.env` before running
anything, so provision scripts see `LIMA_CIDATA_*`: `LIMA_CIDATA_USER`,
`LIMA_CIDATA_UID`, `LIMA_CIDATA_HOME`, `LIMA_CIDATA_NAME`, `LIMA_CIDATA_MNT`,
`LIMA_CIDATA_VMTYPE`, `LIMA_CIDATA_MOUNTS`, `LIMA_CIDATA_CONTAINERD_*`,
`LIMA_CIDATA_PLAIN`, `LIMA_CIDATA_PASSWORDLESS_SUDO`, plus `PARAM_*` from
`param:`. `user` mode additionally gets `XDG_RUNTIME_DIR=/run/user/<uid>`.

They are internal, though: the hostagent scans provision scripts at creation
time and warns `provisioning scripts should not reference the LIMA_CIDATA
variables`. Use the Go template variables above (`{{.User}}`, `{{.Home}}`,
`{{.UID}}`, `{{.Name}}`) instead — `PARAM_*` stays fine.

### Probes

`probes:` (`mode: readiness`) run as the user after provisioning and gate
`limactl start` completion. Each needs a `#!` line; add a `hint:` shown on
failure. Use a probe when a later step (or the operator) depends on a service
actually being up, rather than sleeping inside a provision script. Only
`script` is Go-templated — `hint` is not, so write literal text there.

This repo probes the two things the VM exists for: the rootless Docker daemon
answering `docker info`, and `ssh -T git@github.com` reporting `successfully
authenticated` (that command exits non-zero even on success, so the probe
matches on output).

### Settings file

`~/.config/dev-vm/settings.json` is optional and has exactly two root keys:

```json
{
  "default": {"cpus": 8, "memory": 16, "disk": 100,
              "dotfiles": "git@github.com:user/dotfiles.git",
              "mkcert": true,
              "ghostty": true,
              "clone": [{"org": "gnosispay", "basedir": "${HOME}/Code/gnosispay",
                         "repositories": ["gp-v2"]}]},
  "vms": {"new-vm": {"cpus": 4, "memory": 4, "clone": [],
                     "mkcert": false, "ghostty": false}}
}
```

- `loadSettings(name)` (`devvm.go`) decodes the file and returns one
  `vmConfig`: `default` with `vms.<name>` layered over it by `mergeConfig`.
  `cmdCreate` is its only caller — the settings shape nothing else in the tool
  depends on.
- Every field of `vmConfig` is a **pointer**. That is what separates an absent
  key from an explicit `"clone": []` or `"dotfiles": ""`, both of which must
  turn the default off rather than fall through to it. An override replaces the
  key whole — `clone` lists are not concatenated.
- The decoder runs with `DisallowUnknownFields`, so a typo fails the create
  instead of being ignored, and an old flat settings file (VM keys at the root)
  fails with a hint naming `default` and `vms`. There is no compatibility path:
  such a file must be rewritten.
- Reading the file needs the VM name, which comes from `parseArgs`, so
  `cmdCreate` resolves settings *after* flag parsing — see [VM
  size](#vm-size) for how the flags keep winning anyway.

### Dotfiles

`go run . create -dotfiles REPO` sets the `DOTFILES_REPO` param (via
`--set`),
which `lima/scripts/dotfiles.sh` reads as `PARAM_DOTFILES_REPO` in the guest:
it clones the bare repo to `~/.dotfiles` and checks it out over `$HOME`.
Empty param means the script exits 0 without doing anything.

- The repo can also come from the `dotfiles` key in
  `~/.config/dev-vm/settings.json`, which turns dotfiles on for every VM.
  `-dotfiles REPO` overrides it, `-no-dotfiles` skips it, and a per-VM
  `"dotfiles": ""` turns it back off for that VM alone — see [Settings
  file](#settings-file).
- Pre-existing files the checkout would clobber move to `~/tmp/config-backup`
  keeping their relative path.
- A dotfiles checkout owning `~/.zshrc`/`~/.bashrc` cannot break `DOCKER_HOST`:
  it is exported globally from `/etc/profile.d/docker-host.sh`, reached by
  login shells through `/etc/profile` (zsh via `/etc/zsh/zprofile`) and by
  every other zsh through the line `zsh-system.sh` adds to `/etc/zsh/zshenv`.
- **The dotfiles own `~/.ssh/config` and are responsible for loading every ssh
  config the VM ships.** Nothing in this repo writes that file. The GitHub
  identity (`lima/files/ssh-github.conf`) is staged by `mode: data` at
  `/usr/local/lib/dev-vm/ssh-github.conf` and installed by
  `scripts/ssh-config-user.sh` as `~/.ssh/config.d/10-github.conf`, so the
  dotfiles' own config must carry `Include ~/.ssh/config.d/*.conf` — near the
  top, since ssh keeps the first value per keyword. The directory is created in
  the `user` script, not by `mode: data`: data mode creates missing parents as
  root, which would leave the user unable to drop its own files in.
- Without that `Include` line (or without dotfiles at all) the drop-in is
  inert. The github probe still passes — ssh tries `~/.ssh/id_ed25519` by
  default and `git@github.com` supplies the user — but `IdentitiesOnly yes` is
  not in effect.

### Repository clones

The `clone` key of the resolved settings block (see [Settings
file](#settings-file)) lists repositories to clone in the guest, grouped per
GitHub org:

```json
{"default": {"clone": [{"org": "robsonpeixoto",
                        "basedir": "${HOME}/Code/robsonpeixoto",
                        "repositories": ["dev-vm", "echo-server"]}]}}
```

- `settingsClones` (`create.go`) validates and flattens it to one
  `{basedir, org/repo}` entry per repository; `cloneList` renders the guest
  list, tab-separated, one line per repository behind a comment header. The
  header is not decoration: it keeps the file non-empty when nothing is
  configured, since the `mode: data` entry for it is unconditional.
- `startVM` writes it to `tmp/clone-list` in the materialized template tree —
  the same trick as the private key at `tmp/default` — and `mode: data` stages
  it at `/usr/local/lib/dev-vm/clone-list`.
- `scripts/clone-user.sh` is the **last** `user` script: cloning needs the
  provisioned key, `known_hosts` and git from the steps before it. It expands
  `${HOME}`/`$HOME` in `basedir` in the guest (the host cannot: that path is
  the guest home), skips a repository whose `<basedir>/<repo>` already exists,
  and logs-and-skips one that fails to clone so a single unreachable repo does
  not take the rest down.
- No flag configures this; the setting is the only input, and it is read at
  create time like everything else in the template. A per-VM `"clone": []`
  clones nothing for that VM while the default block stays intact for the
  others.

### mkcert root CA

`"mkcert": true` in the resolved settings block copies the host mkcert CA into
the guest, so a certificate issued on either side is trusted on both.

- `resolveCAROOT` (`create.go`) runs `mkcert -CAROOT` on the host and requires
  both `rootCA.pem` and `rootCA-key.pem` to be there — the setting asked for
  the CA, so a missing binary or a missing file is a `die`, not a silent skip.
  The key is copied along with the certificate on purpose: without it the guest
  can trust the CA but not issue from it.
- `startVM` writes both files into `tmp/` of the materialized template tree,
  the same trick as the private key and the clone list, and two unconditional
  `mode: data` entries stage them at `/usr/local/lib/dev-vm/`. With the setting
  off the files are written empty, which `mkcert-user.sh` treats as "do
  nothing". `rootCA-key.pem` is staged `owner: "{{.User}}"`, `permissions: 600`
  — a root-only file would be unreadable by the user script that installs it.
- `scripts/mkcert-user.sh` runs after `mise-user.sh` (mkcert usually comes from
  mise, so by then `mkcert -CAROOT` can answer for itself) and installs the
  pair into `$CAROOT`, else `${XDG_DATA_HOME:-$HOME/.local/share}/mkcert`.
  It must not write into `$HOME` through `mode: data`: `~/.local/share` also
  holds mise's own tree, and data mode would create it root-owned.
- `startSet` adds `.caCerts.files = ["<host CAROOT>/rootCA.pem"]`, which
  cloud-init installs into `/usr/local/share/ca-certificates` and trusts with
  `update-ca-certificates`. That is a **host** path stored in the instance's
  `lima.yaml`, and the hostagent re-reads it on every start when it regenerates
  `cidata.iso` — so the temp tree copy would not do, and removing the host
  CAROOT later breaks `devvm start` for that VM.
- Nothing runs `mkcert -install` in the guest: `caCerts` covers the system
  store, and the browser NSS store is the operator's business
  (`libnss3-tools` is installed by `packages-system.sh`).

### ghostty terminfo

`"ghostty": true` compiles the `xterm-ghostty` terminfo entry in the guest, so
the `TERM` Ghostty exports survives `limactl shell` and ssh.

- `hostTerminfo` (`create.go`) runs `$(brew --prefix ncurses)/bin/infocmp -x
  xterm-ghostty`. **Homebrew's infocmp, not the macOS one**: `/usr/bin/infocmp`
  is ncurses 6.0.20150808 and mangles the extended (`-x`) capabilities that are
  most of the entry. A missing binary is a `die` naming
  `brew install ncurses`.
- The entry is looked up through `TERMINFO`, which Ghostty sets to its app
  bundle. `create` may run outside Ghostty, so a failed dump is retried with
  `TERMINFO=/Applications/Ghostty.app/Contents/Resources/terminfo`.
- The dump is staged **base64-encoded**, via `terminfoB64`. That is not
  cosmetic: Lima runs `mode: data` content *and* provision scripts through a Go
  template on the host, and the entry's `acsc` capability lists the ACS pairs
  for the box-drawing characters, doubled braces among them. The template
  parser reads those as an action and rejects the file
  (`unexpected "|" in command`); Lima logs
  `Couldn't process data content as a template` and the payload survives only
  by falling back to the unparsed text. The same trap caught a *comment* in
  `ghostty-terminfo-system.sh` that quoted the braces literally — that script
  failed to parse with `unterminated raw quoted string`. Never put doubled
  braces in a provision script, comments included.
- `scripts/ghostty-terminfo-system.sh` is a **system** script (after
  `packages-system.sh`): `base64 -d | tic -x -o /usr/share/terminfo -`, so root
  and sudo shells resolve the entry too, not just the login user. It installs
  `ncurses-bin` when `tic` is missing, and skips an empty staged file.
- Nothing sets `TERM` in the guest; ssh and `limactl shell` carry the host
  value.

### VM size

`cpus`, `memory` and `disk` are **top-level template fields**, not params, so
`devvm create` patches them with `.cpus = N | .memory = "NGiB" | .disk = "NGiB"`
rather than `.param.*`. Resolution order: `resolveResources` (`create.go`)
starts from `defaultResources` (2 vCPUs, 2 GiB, 50 GiB), applies the
`cpus`/`memory`/`disk` keys of the resolved settings block, then applies
whichever of `-cpus`/`-memory`/`-disk` were actually passed. The values in
`lima/dev-vm.yaml` are documentation only — `--set` always overwrites them.

The flags are registered with a **zero** default and the settings are applied
after parsing, not before: the settings block depends on the VM name, and the
name only exists once `parseArgs` has run. `flagsSet` (`fs.Visit`) is what
tells an unset flag from `-cpus 0`, so the zero default never leaks into the
template.

Flags and settings are integers in GiB; non-integers are rejected by the `flag`
package and non-positive values by `resolveResources` (settings) and
`checkResources` (flags), all before the VM starts. Because the template is
flattened at creation, the size is fixed for the instance's life: resizing
means `limactl edit` or delete + create.

`go run . list` reads the live `cpus`/`memory`/`disk` back out of
`limactl list --format json`, where memory and disk are **bytes**. That is the
declared *size* only — Lima reports no usage figure, so disk pressure is a
guest question (`docker system df`, `df -h /`) and `list` deliberately stays out
of it. Nothing prunes the disk on a schedule — the VM runs no cron at all — so
reclaiming space is a manual `docker system prune`; see the disk hygiene
section of README.md.

### Guest OS pin

`base:` in `lima/dev-vm.yaml` names one release —
`template:_images/ubuntu-26.04` — and never `template:_images/ubuntu-lts`.
`ubuntu-lts.yaml` is a **symlink shipped inside the installed Lima**
(in v2.2.0 it points at `ubuntu-26.04.yaml`), so it resolves against whichever
Lima is on the host: a Lima upgrade would move new VMs to the next LTS with no
change in this repo.

- Verify a template name without booting anything — it must exist as
  `_images/ubuntu-XX.YY.yaml` in `$(brew --prefix lima)/share/lima/templates/`
  (or under `templates/_images` at the matching upstream tag).
- `base` is merged at creation, so changing the pin affects only VMs created
  afterwards; existing ones keep their release for life.
- The bump procedure and its new-LTS checklist (boot, docker probe, github
  probe) live in the "Guest OS" section of README.md. Gate on Docker's apt repo
  carrying the new codename — `docker-system.sh` derives it from the guest's
  `VERSION_CODENAME`, so bumping first breaks provisioning at `apt-get update`.

### Debugging

- Host: `~/.lima/<name>/ha.stderr.log`, `serial*.log`.
- Guest: `/var/log/cloud-init-output.log` — provision script stdout/stderr,
  each prefixed by `LIMA <timestamp>| Executing /mnt/lima-cidata/provision.<mode>/<NNNNNNNN>`.
- Re-run provisioning without recreating: `limactl stop <name> && limactl start <name>`.
- Inspect what actually shipped: `limactl shell <name> sudo ls /mnt/lima-cidata`
  (root-only, mode 0700) and `~/.lima/<name>/lima.yaml` on the host.

### Writing provision scripts for this repo

- Put executable logic in `lima/scripts/*.sh`, reference it via `file.url`.
- `#!/bin/sh` + `set -eux` (or `set -eu` when output would leak secrets).
- Indent with 4 spaces, not tabs, and run `make format` before committing —
  never a bare `shfmt`/`gofmt`, so the flags stay in one place. The `Makefile`
  holds `format`/`check-format` and their `-go`/`-shell` halves; `SHELL_DIRS`
  (`lima completions`) is the list shfmt walks, finding shell files by extension
  or shebang, so the extensionless `lima/files/install-neovim` needs no
  listing. The width comes
  from the repo-root `.editorconfig` (`root = true`, so it covers every
  directory) and its `[[shell]]` section, an shfmt extension that matches by
  shebang too. CI's `check` job runs `make check-format-go`, its `shell` job
  `make check-format-shell` plus `shellcheck`, including
  `-s bash completions/dev-vm.bash`; the zsh and fish completions are dialects
  shellcheck cannot read.
- Idempotent, always — the script reruns on every boot.
- Root work in `system`, per-user/systemd work in `user`, never mix.
- No `-o DPkg::Lock::Timeout` anywhere. apt already defaults
  `Dpkg::Lock::Timeout` to 120 s whenever stdin is not a tty
  (`BinarySpecificConfiguration` in apt's `private-cmndline.cc`, since apt
  2.0), which covers every non-interactive run, and the flag never applied to
  `apt-get update` in the first place: the lists lock goes through
  `pkgAcquire::GetLock` → `GetLock` (a non-blocking `F_SETLK`), not
  `GetLockMaybeWait`. Contention is solved by having no second apt scheduler —
  see the no-auto-upgrade note below — not by passing the option.
- Secrets and config files go through `mode: data` with explicit `owner` and
  `permissions`, not `echo` or a heredoc inside a script. Static payloads live
  in `lima/files/`, referenced with `file.url` like the scripts.
- **Nothing in the guest upgrades itself, and nothing runs on a schedule.**
  There is no cron, no `cron.d`, no `unattended-upgrades`. Boot provisioning
  installs what is missing and stops there; upgrading is the operator's
  `apt-get upgrade` (plus `/usr/local/lib/dev-vm/install-neovim` for neovim,
  which apt does not carry). Two pieces enforce it and must stay:
  `scripts/no-auto-upgrades-system.sh` **masks** `apt-daily.timer`,
  `apt-daily-upgrade.timer`, both of their services and
  `unattended-upgrades.service` (the timers fire in a randomized window and,
  being `Persistent=true`, catch up right after a VM start, next to boot
  provisioning), and the `mode: data` file
  `lima/files/apt/99dev-vm-no-auto-upgrades` zeros every `APT::Periodic` key.
  Mask, never merely disable: an apt upgrade re-enables a disabled unit, and a
  masked unit cannot be started even while it is still enabled.
- Do not reintroduce a scheduler. A recurring job would be a second apt client
  racing boot provisioning for the dpkg lock, and version drift nobody asked
  for. Anything periodic belongs on the host side of the VM, or in the
  operator's hands.
- Templates, scripts and static files are embedded into the Go binary with
  `//go:embed` (see `embed.go`) and materialized into a temp dir at create
  time; `go run .` picks up edits automatically, a prebuilt `devvm` binary
  must be rebuilt.
- After changing a template or script, the VM must be recreated
  (`go run . delete <name> && go run . create <name>`) for the change to
  apply.
