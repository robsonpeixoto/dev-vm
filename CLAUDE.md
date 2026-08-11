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
  (`go run . destroy && go run . create`) or `limactl edit <name>`.
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
  `overwrite: false` to write only if absent). Parent directories are created
  as the owner user. reverse-sshfs mounts are not up yet at this point.
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

### Dotfiles

`go run . create -dotfiles REPO` sets the `DOTFILES_REPO` param (via
`--set`),
which `lima/scripts/dotfiles.sh` reads as `PARAM_DOTFILES_REPO` in the guest:
it clones the bare repo to `~/.dotfiles` and checks it out over `$HOME`.
Empty param means the script exits 0 without doing anything.

- The repo can also come from `{"dotfiles": "<repo>"}` in
  `~/.config/dev-vm/settings.json`, which turns dotfiles on for every VM.
  `-dotfiles REPO` overrides it, `-no-dotfiles` skips it.
- Pre-existing files the checkout would clobber move to `~/tmp/config-backup`
  keeping their relative path.
- A dotfiles checkout owning `~/.zshrc`/`~/.bashrc` cannot break `DOCKER_HOST`:
  it is exported globally from `/etc/profile.d/docker-host.sh`, reached by
  login shells through `/etc/profile` (zsh via `/etc/zsh/zprofile`) and by
  every other zsh through the line `zsh-system.sh` adds to `/etc/zsh/zshenv`.
- A dotfiles repo carrying `.ssh/config` replaces the provisioned one, so
  `dotfiles.sh` prepends the GitHub stanza back from
  `~/.ssh/lima-github.conf` (same `mode: data` payload,
  `lima/files/ssh-github.conf`); ssh keeps the first value per keyword.

### VM size

`cpus`, `memory` and `disk` are **top-level template fields**, not params, so
`devvm create` patches them with `.cpus = N | .memory = "NGiB" | .disk = "NGiB"`
rather than `.param.*`. Resolution order: `settingsResources` (`create.go`)
starts from `defaultResources` (2 vCPUs, 2 GiB, 50 GiB) and applies the
`cpus`/`memory`/`disk` keys in `~/.config/dev-vm/settings.json`; that result is
the default handed to the `-cpus`/`-memory`/`-disk` `fs.IntVar` flags, so a flag
that is passed wins. The values in `lima/dev-vm.yaml` are documentation only —
`--set` always overwrites them.

Flags and settings are integers in GiB; non-integers are rejected by the `flag`
package and non-positive values by `checkResources`, both before the VM starts. Because the template is flattened at creation, the size is fixed for
the instance's life: resizing means `limactl edit` or destroy + create.

`go run . list` reads the live `cpus`/`memory`/`disk` back out of
`limactl list --format json`, where memory and disk are **bytes**.

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
- Idempotent, always — the script reruns on every boot.
- Root work in `system`, per-user/systemd work in `user`, never mix.
- Every `apt-get` call takes `-o DPkg::Lock::Timeout=120`. Provision scripts,
  the Docker updater cron job and Ubuntu's unattended-upgrades all compete for
  the dpkg lock on boot; without the timeout apt-get fails at once and
  `set -eux` turns the lost race into a provisioning failure.
- Secrets and config files go through `mode: data` with explicit `owner` and
  `permissions`, not `echo` or a heredoc inside a script. Static payloads live
  in `lima/files/`, referenced with `file.url` like the scripts.
- Templates, scripts and static files are embedded into the Go binary with
  `//go:embed` (see `embed.go`) and materialized into a temp dir at create
  time; `go run .` picks up edits automatically, a prebuilt `devvm` binary
  must be rebuilt.
- After changing a template or script, the VM must be recreated
  (`go run . destroy <name> && go run . create <name>`) for the change to
  apply.
