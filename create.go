package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const createUsage = `Create an isolated Lima dev VM with SSH access to GitHub.

Usage: devvm create [name] [--create-ssh-key[=no]] [--dotfiles[=REPO]|--no-dotfiles]
                    [-cpus N] [-memory GiB] [-disk GiB]

- Generates a fresh ed25519 key pair at ~/.config/dev-vm/keys/<name> (no
  passphrase) on every create; --create-ssh-key=no reuses the existing pair.
- Registers the public key on GitHub with title <name> via gh, replacing
  any key recorded in the state file or sharing the same title.
- Starts the VM from the embedded lima/dev-vm.yaml; the template uploads the
  private key and ssh config into the guest, and provisioning fetches
  known_hosts.
- With dotfiles enabled, provisioning clones the bare repo to ~/.dotfiles in
  the guest and checks it out over $HOME. The repo comes from --dotfiles or
  from {"dotfiles": "<repo>"} in ~/.config/dev-vm/settings.json, which also
  turns dotfiles on by default for every VM.
- Sizes the VM at 2 vCPUs, 2 GiB RAM and a 50 GiB disk by default. -cpus,
  -memory and -disk override that; -memory and -disk are plain integers in
  GiB. The "cpus", "memory" and "disk" entries in
  ~/.config/dev-vm/settings.json change the defaults for every VM. Size is
  fixed at create time — resizing means destroy and create again.
- Records VM metadata (GitHub key id, key paths) in ~/.config/dev-vm/state.json.

Flags taking an optional value need the = form: --dotfiles=REPO.

`

// optFlag is a string flag whose value is optional: IsBoolFlag lets the flag
// package accept a bare --name (Set receives "true") as well as --name=value.
type optFlag struct {
	set   bool
	value string
}

func (o *optFlag) String() string { return o.value }

func (o *optFlag) IsBoolFlag() bool { return true }

func (o *optFlag) Set(s string) error {
	o.set = true
	if s == "true" {
		s = ""
	}
	o.value = s
	return nil
}

// intFlag is a positive-integer flag that records whether it was set, so
// resolution can fall back to settings.json.
type intFlag struct {
	set   bool
	value int
}

func (i *intFlag) String() string {
	if !i.set {
		return ""
	}
	return strconv.Itoa(i.value)
}

func (i *intFlag) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return errors.New("must be a positive integer")
	}
	i.set, i.value = true, n
	return nil
}

// resources is the VM size; memory and disk are in GiB.
type resources struct {
	cpus   int
	memory int
	disk   int
}

var defaultResources = resources{cpus: 2, memory: 2, disk: 50}

func cmdCreate(argv []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, createUsage)
		fs.PrintDefaults()
	}
	var sshKey, dotfilesFlag optFlag
	var noDotfiles bool
	fs.Var(&sshKey, "create-ssh-key",
		"yes (default): create and register a new key; no: reuse existing key")
	fs.Var(&dotfilesFlag, "dotfiles",
		"clone REPO as a bare repo and check it out over the guest $HOME; "+
			"without =REPO, use the \"dotfiles\" entry in settings.json")
	fs.BoolVar(&noDotfiles, "no-dotfiles", false,
		"skip dotfiles even when settings.json configures them")
	var cpus, memory, disk intFlag
	fs.Var(&cpus, "cpus", "vCPUs for the VM (default 2, or \"cpus\" in settings.json)")
	fs.Var(&memory, "memory", "RAM in GiB (default 2, or \"memory\" in settings.json)")
	fs.Var(&disk, "disk", "disk size in GiB (default 50, or \"disk\" in settings.json)")
	name := parseArgs(fs, argv)

	checkName(name)
	dotfiles := resolveDotfiles(dotfilesFlag, noDotfiles)
	res := resolveResources(cpus, memory, disk)
	if vmExists(name) {
		die("VM %q already exists; run: devvm destroy %s", name, name)
	}

	key, pub := keyPaths(name)
	mode := sshKeyMode(sshKey)

	if mode == "no" {
		if !fileExists(key) || !fileExists(pub) {
			die("no key at %s; rerun with --create-ssh-key", key)
		}
		fmt.Printf("reusing key %s\n", key)
	} else {
		checkScopes()
		createKey(name, key, pub)
		keyID := registerKey(name, pub)
		putVM(name, map[string]any{
			"github_key_id":    keyID,
			"github_key_title": name,
			"private_key":      key,
			"public_key":       pub,
		})
	}

	if dotfiles != "" {
		fmt.Printf("installing dotfiles from %s\n", dotfiles)
	}
	fmt.Printf("VM size %d vCPU, %dGiB RAM, %dGiB disk\n", res.cpus, res.memory, res.disk)
	startVM(name, dotfiles, res, key)
	putVM(name, map[string]any{
		"template":    "embedded:lima/dev-vm.yaml",
		"started_at":  now(),
		"private_key": key,
		"public_key":  pub,
		"dotfiles":    dotfiles,
	})
	fmt.Printf("state %s\n", stateFile)
}

// parseArgs parses flags, then an optional positional name, then any flags
// placed after it.
func parseArgs(fs *flag.FlagSet, argv []string) string {
	fs.Parse(argv)
	args := fs.Args()
	if len(args) == 0 {
		return "default"
	}
	name := args[0]
	fs.Parse(args[1:])
	if fs.NArg() > 0 {
		die("unexpected argument %q", fs.Arg(0))
	}
	return name
}

func sshKeyMode(sshKey optFlag) string {
	switch {
	case !sshKey.set, sshKey.value == "", sshKey.value == "yes":
		return "yes"
	case sshKey.value == "no":
		return "no"
	}
	die("--create-ssh-key must be yes or no, got %q", sshKey.value)
	return ""
}

// resolveDotfiles: CLI wins over settings.json; bare --dotfiles falls back to
// settings.
func resolveDotfiles(dotfiles optFlag, noDotfiles bool) string {
	if noDotfiles {
		return ""
	}
	configured, _ := loadSettings()["dotfiles"].(string)
	repo := configured
	if dotfiles.set && dotfiles.value != "" {
		repo = dotfiles.value
	}
	if dotfiles.set && dotfiles.value == "" && repo == "" {
		die("--dotfiles needs a repo, or a \"dotfiles\" entry in %s", settingsFile)
	}
	if repo != "" {
		checkRepo(repo)
	}
	return repo
}

// resolveResources: CLI wins over settings.json, which wins over the built-in
// defaults.
func resolveResources(cpus, memory, disk intFlag) resources {
	settings := loadSettings()
	res := defaultResources
	for _, r := range []struct {
		key  string
		flag intFlag
		out  *int
	}{
		{"cpus", cpus, &res.cpus},
		{"memory", memory, &res.memory},
		{"disk", disk, &res.disk},
	} {
		if v, ok := settings[r.key]; ok {
			*r.out = settingsInt(r.key, v)
		}
		if r.flag.set {
			*r.out = r.flag.value
		}
	}
	return res
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func createKey(name, key, pub string) {
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		die("cannot create %s: %v", keyDir, err)
	}
	if err := os.Chmod(keyDir, 0o700); err != nil {
		die("cannot chmod %s: %v", keyDir, err)
	}
	for _, p := range []string{key, pub} {
		os.Remove(p)
	}
	run("ssh-keygen", "-t", "ed25519", "-N", "", "-C", name, "-f", key)
	if err := os.Chmod(key, 0o600); err != nil {
		die("cannot chmod %s: %v", key, err)
	}
	fmt.Printf("created key %s\n", key)
}

func registerKey(name, pub string) int64 {
	previous, _ := keyID(getVM(name)["github_key_id"])
	pubData, err := os.ReadFile(pub)
	if err != nil {
		die("cannot read %s: %v", pub, err)
	}
	for _, k := range listKeys() {
		if k.Title == name || k.ID == previous {
			deleteKey(k.ID)
			fmt.Printf("deleted old GitHub key %d (%s)\n", k.ID, k.Title)
		}
	}
	created := addKey(name, strings.TrimSpace(string(pubData)))
	fmt.Printf("registered GitHub key %d (%s)\n", created.ID, name)
	return created.ID
}

// startVM materializes the embedded template tree into a temp directory —
// limactl resolves provision file.url paths relative to the template — drops
// the private key at tmp/default where the template expects it, and boots.
func startVM(name, dotfiles string, res resources, key string) {
	dir, err := os.MkdirTemp("", "dev-vm-")
	if err != nil {
		die("cannot create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	err = fs.WalkDir(assets, "lima", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, strings.TrimPrefix(path, "lima/"))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := assets.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		die("cannot materialize template: %v", err)
	}

	keyData, err := os.ReadFile(key)
	if err != nil {
		die("cannot read %s: %v", key, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o700); err != nil {
		die("cannot materialize template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmp", "default"), keyData, 0o600); err != nil {
		die("cannot materialize template: %v", err)
	}

	limactlRun("start", "--tty=false", "--name", name,
		"--set", fmt.Sprintf(
			`.cpus = %d | .memory = "%dGiB" | .disk = "%dGiB" | .param.DOTFILES_REPO = %q`,
			res.cpus, res.memory, res.disk, dotfiles),
		filepath.Join(dir, "dev-vm.yaml"))
}
