package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const recreateUsage = `Rebuild a dev VM from the current template, keeping the guest home directory.

Usage: devvm recreate [name] [-create-ssh-key=false] [-force]
                      [-cpus N] [-memory GiB] [-disk GiB]

Same flags as devvm create, minus -dotfiles/-no-dotfiles: the dotfiles repo is
the one the VM already uses, read from ~/.config/dev-vm/state.json.

- Archives the guest home directory to
  ~/.config/dev-vm/backups/<name>-<timestamp>.tar.gz, starting the VM first
  when it is not running. The archive is kept afterwards.
- Deletes the Lima VM, creates it again from the current embedded template and
  extracts the archive back over the new home directory. Dotfiles, gh and
  claude credentials, shell history and everything else under $HOME survive;
  the rest of the guest (installed packages, docker images, /etc edits) does
  not.
- Leaves out of the archive what provisioning owns and rewrites: the VM's ssh
  key and config, Lima's authorized_keys, ~/.cache, and the rootless docker
  data in ~/.local/share/docker.
- Defaults the VM size to the instance's current size rather than to
  settings.json, so a rebuild keeps the size the VM already has; -cpus,
  -memory and -disk override it.
- Registers a fresh GitHub key like create does, replacing the VM's old one;
  -create-ssh-key=false reuses the existing pair.

`

// homeExcludes are the home directory paths provisioning owns, left out of the
// archive so the rebuilt VM keeps its own ssh identity and docker state. Paths
// are relative to $HOME, matching the "." tar walks.
var homeExcludes = []string{
	"./.ssh/id_ed25519",
	"./.ssh/id_ed25519.pub",
	"./.ssh/config",
	"./.ssh/lima-github.conf",
	"./.ssh/authorized_keys",
	"./.cache",
	"./.local/share/docker",
}

// minArchiveSize guards against destroying a VM over a truncated archive: an
// empty gzip stream is 20 bytes, a real home directory is far bigger.
const minArchiveSize = 1024

func cmdRecreate(argv []string) {
	fs := flag.NewFlagSet("recreate", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, recreateUsage)
		fs.PrintDefaults()
	}
	var createSSHKey bool
	var force bool
	fs.BoolVar(&createSSHKey, "create-ssh-key", true,
		"create and register a new key; -create-ssh-key=false reuses the existing key")
	fs.BoolVar(&force, "force", false,
		"skip the confirmation prompt; required when stdin is not a terminal")
	// Zero means unset: the default comes from the live instance, which is not
	// known until the name is parsed.
	var flags resources
	fs.IntVar(&flags.cpus, "cpus", 0, "vCPUs for the VM (default: its current size)")
	fs.IntVar(&flags.memory, "memory", 0, "RAM in GiB (default: its current size)")
	fs.IntVar(&flags.disk, "disk", 0, "disk size in GiB (default: its current size)")
	name := parseArgs(fs, argv)

	checkName(name)
	inst, exists := limaInstances()[name]
	if !exists {
		die("no VM %q to recreate; run: devvm create %s", name, name)
	}
	res := recreateResources(flags, inst)
	checkResources(res)

	dotfiles, _ := getVM(name)["dotfiles"].(string)
	if dotfiles != "" {
		checkRepo(dotfiles)
	}
	// Before anything is deleted: an auth problem must cost nothing.
	if createSSHKey {
		checkScopes()
	}
	if !force {
		if !isTerminal(os.Stdin) {
			die("recreate needs a terminal to confirm; rerun with: devvm recreate %s -force", name)
		}
		if err := confirmRecreate(name, os.Stdin, os.Stdout); err != nil {
			die("%v", err)
		}
	}

	archive := backupHome(name)
	deleteVM(name)
	createVM(createOptions{
		name:         name,
		dotfiles:     dotfiles,
		res:          res,
		createSSHKey: createSSHKey,
	})
	restoreHome(name, archive)
	putVM(name, map[string]any{
		"home_backup":  archive,
		"recreated_at": now(),
	})
}

// recreateResources resolves the VM size: a flag that was passed wins, anything
// unset keeps the instance's current size, and a size Lima does not report
// falls back to the create defaults.
func recreateResources(flags resources, inst limaInstance) resources {
	base := settingsResources()
	if inst.CPUs > 0 {
		base.cpus = inst.CPUs
	}
	if inst.Memory > 0 {
		base.memory = int(inst.Memory >> 30)
	}
	if inst.Disk > 0 {
		base.disk = int(inst.Disk >> 30)
	}
	for _, r := range []struct{ flag, base *int }{
		{&flags.cpus, &base.cpus},
		{&flags.memory, &base.memory},
		{&flags.disk, &base.disk},
	} {
		if *r.flag == 0 {
			*r.flag = *r.base
		}
	}
	return flags
}

// confirmRecreate asks for a plain yes: the home directory survives, so this
// is a lighter prompt than destroy's type-the-name. A short read is a no.
func confirmRecreate(name string, in io.Reader, out io.Writer) error {
	fmt.Fprintf(out, "This deletes VM %q and builds it again. The guest home directory is "+
		"archived and restored; everything else in the guest is lost.\n", name)
	fmt.Fprintf(out, "Recreate %s? [y/N]: ", name)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("cannot read confirmation: %v", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return errors.New("not confirmed, nothing changed")
}

// backupHome streams a tar of the guest home directory to a file on the host,
// starting the VM when needed. It dies without leaving an archive behind
// unless the archive looks usable, since the caller deletes the VM next.
func backupHome(name string) string {
	ensureRunning(name)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		die("cannot create %s: %v", backupDir, err)
	}
	path := filepath.Join(backupDir, fmt.Sprintf("%s-%s.tar.gz", name, stamp()))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		die("cannot create %s: %v", path, err)
	}

	fmt.Printf("archiving guest home of %s\n", name)
	runErr := limactlPipe(nil, f, "shell", "--workdir", "/", name,
		"sh", "-c", archiveCommand())
	closeErr := f.Close()
	// tar exits 1 when a file changed underneath it, which a live guest does.
	if runErr != nil && exitCode(runErr) != 1 {
		os.Remove(path)
		die("cannot archive the guest home of %s: %v", name, runErr)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "warning: tar reported changed files while archiving")
	}
	if closeErr != nil {
		die("cannot write %s: %v", path, closeErr)
	}
	info, err := os.Stat(path)
	if err != nil {
		die("cannot read %s: %v", path, err)
	}
	if info.Size() < minArchiveSize {
		os.Remove(path)
		die("archive of the guest home is empty; nothing destroyed")
	}
	fmt.Printf("archived %s (%s)\n", path, mib(info.Size()))
	return path
}

// archiveCommand builds the guest side of the backup: tar the home directory
// to stdout, skipping the paths provisioning rewrites.
func archiveCommand() string {
	var b strings.Builder
	b.WriteString(`tar -C "$HOME" -czf -`)
	for _, e := range homeExcludes {
		fmt.Fprintf(&b, " --exclude=%s", e)
	}
	b.WriteString(" .")
	return b.String()
}

// restoreHome extracts the archive over the freshly provisioned home
// directory, so the old contents win wherever both have a file.
func restoreHome(name, archive string) {
	f, err := os.Open(archive)
	if err != nil {
		die("cannot read %s: %v", archive, err)
	}
	defer f.Close()

	fmt.Printf("restoring guest home from %s\n", archive)
	err = limactlPipe(f, os.Stdout, "shell", "--workdir", "/", name,
		"sh", "-c", `tar -C "$HOME" -xzf -`)
	if err != nil && exitCode(err) != 1 {
		die("cannot restore the guest home from %s: %v", archive, err)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: tar reported problems while restoring")
	}
	fmt.Printf("home restored; archive kept at %s\n", archive)
}

// ensureRunning boots the VM when it is not running, since the home directory
// can only be read from inside the guest.
func ensureRunning(name string) {
	if inst, ok := limaInstances()[name]; ok && inst.Status == "Running" {
		return
	}
	fmt.Printf("starting %s to read its home directory\n", name)
	limactlRun("start", "--tty=false", name)
}
