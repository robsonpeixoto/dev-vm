package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const createUsage = `Create an isolated Lima dev VM with SSH access to GitHub.

Usage: devvm create [name] [-create-ssh-key=false] [-dotfiles REPO|-no-dotfiles]
                    [-cpus N] [-memory GiB] [-disk GiB]

- Generates a fresh ed25519 key pair at ~/.config/dev-vm/keys/<name> (no
  passphrase) on every create; -create-ssh-key=false reuses the existing pair.
- Registers the public key on GitHub with title dev-vm/<host>/<name> via gh,
  replacing any key recorded in the state file or sharing the same title. The
  host qualifier keeps two machines using the same VM name from deleting each
  other's key.
- Starts the VM from the embedded lima/dev-vm.yaml; the template uploads the
  private key and ssh config into the guest, and provisioning fetches
  known_hosts.
- Reads ~/.config/dev-vm/settings.json for the "dotfiles", "cpus", "memory",
  "disk", "clone" and "mkcert" keys: the "default" block applies to every VM, and
  "vms".<name> overrides it key by key for this one.
- With dotfiles enabled, provisioning clones the bare repo to ~/.dotfiles in
  the guest and checks it out over $HOME. The repo comes from -dotfiles or
  from the "dotfiles" setting, which turns dotfiles on by default.
- Sizes the VM at 2 vCPUs, 2 GiB RAM and a 50 GiB disk by default. -cpus,
  -memory and -disk override both that and the settings; -memory and -disk are
  plain integers in GiB. Size is fixed at create time — resizing means delete
  and create again.
- Clones the repositories listed under "clone" as the last user provisioning
  step; a repository already present in the guest is skipped.
- With "mkcert": true in the settings, copies the host mkcert root CA
  (rootCA.pem and rootCA-key.pem from mkcert -CAROOT) into the guest CAROOT
  and trusts rootCA.pem in the guest system store through the template's
  caCerts.files.
- Records VM metadata (GitHub key id, key paths) in ~/.config/dev-vm/state.json.

`

// resources is the VM size; memory and disk are in GiB.
type resources struct {
	cpus   int
	memory int
	disk   int
}

var defaultResources = resources{cpus: 2, memory: 2, disk: 50}

// caFiles are the mkcert root CA files copied from the host CAROOT into the
// guest one. The key comes along so the guest can issue its own certificates.
var caFiles = []string{"rootCA.pem", "rootCA-key.pem"}

// cloneRepo is one repository the guest clones, with the directory it goes
// under. repo is "<org>/<name>".
type cloneRepo struct {
	basedir string
	repo    string
}

func cmdCreate(argv []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, createUsage)
		fs.PrintDefaults()
	}
	var createSSHKey bool
	var dotfilesRepo string
	var noDotfiles bool
	fs.BoolVar(&createSSHKey, "create-ssh-key", true,
		"create and register a new key; -create-ssh-key=false reuses the existing key")
	fs.StringVar(&dotfilesRepo, "dotfiles", "",
		"clone REPO as a bare repo and check it out over the guest $HOME; "+
			"unset, use the \"dotfiles\" entry in settings.json")
	fs.BoolVar(&noDotfiles, "no-dotfiles", false,
		"skip dotfiles even when settings.json configures them")
	// Zero defaults: the real ones come from settings.json, which can only be
	// read once the VM name is known. resolveResources fills in what no flag set.
	var res resources
	fs.IntVar(&res.cpus, "cpus", 0, "vCPUs for the VM (default 2, or settings.json)")
	fs.IntVar(&res.memory, "memory", 0, "RAM in GiB (default 2, or settings.json)")
	fs.IntVar(&res.disk, "disk", 0, "disk size in GiB (default 50, or settings.json)")
	name := parseArgs(fs, argv)

	checkName(name)
	config := loadSettings(name)
	res = resolveResources(res, flagsSet(fs), config)
	checkResources(res)
	dotfiles := resolveDotfiles(dotfilesRepo, noDotfiles, config)
	clones := settingsClones(config)
	caroot := resolveCAROOT(config)
	if vmExists(name) {
		die("VM %q already exists; run: devvm delete %s", name, name)
	}

	key, pub := keyPaths(name)

	if !createSSHKey {
		if !fileExists(key) || !fileExists(pub) {
			die("no key at %s; rerun without -create-ssh-key=false", key)
		}
		fmt.Printf("reusing key %s\n", key)
	} else {
		checkScopes()
		title := keyTitle(name)
		createKey(title, key, pub)
		keyID := registerKey(name, title, pub)
		putVM(name, map[string]any{
			"github_key_id":    keyID,
			"github_key_title": title,
			"private_key":      key,
			"public_key":       pub,
		})
	}

	if dotfiles != "" {
		fmt.Printf("installing dotfiles from %s\n", dotfiles)
	}
	if len(clones) > 0 {
		fmt.Printf("cloning %d repositories\n", len(clones))
	}
	if caroot != "" {
		fmt.Printf("copying the mkcert root CA from %s\n", caroot)
	}
	fmt.Printf("VM size %d vCPU, %dGiB RAM, %dGiB disk\n", res.cpus, res.memory, res.disk)
	startVM(name, dotfiles, res, key, clones, caroot)
	putVM(name, map[string]any{
		"template":    "embedded:lima/dev-vm.yaml",
		"started_at":  now(),
		"private_key": key,
		"public_key":  pub,
		"dotfiles":    dotfiles,
	})
	if inst, ok := limaInstances()[name]; ok {
		if ip := guestIP(inst); ip != "" {
			fmt.Printf("IP %s (no port forwards; reach guest services here)\n", ip)
		}
	}
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

// resolveDotfiles: -dotfiles wins over settings.json; unset falls back to
// settings.
func resolveDotfiles(repo string, noDotfiles bool, config vmConfig) string {
	if noDotfiles {
		return ""
	}
	if repo == "" && config.Dotfiles != nil {
		repo = *config.Dotfiles
	}
	if repo != "" {
		checkRepo(repo)
	}
	return repo
}

// resolveCAROOT returns the host mkcert CA directory when the "mkcert" setting
// is on, and "" otherwise.
func resolveCAROOT(config vmConfig) string {
	if !mkcertEnabled(config) {
		return ""
	}
	return hostCAROOT()
}

func mkcertEnabled(config vmConfig) bool {
	return config.Mkcert != nil && *config.Mkcert
}

// hostCAROOT asks mkcert where its CA lives and checks both files are there;
// the settings asked for the CA, so a missing one is an error rather than a
// silent skip.
func hostCAROOT() string {
	out, err := exec.Command("mkcert", "-CAROOT").Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			die("mkcert not found; install it or unset \"mkcert\" in %s", settingsFile)
		}
		die("mkcert -CAROOT failed: %v", err)
	}
	caroot := strings.TrimSpace(string(out))
	if caroot == "" {
		die("mkcert -CAROOT printed nothing")
	}
	for _, f := range caFiles {
		if !fileExists(filepath.Join(caroot, f)) {
			die("no %s in %s; run: mkcert -install", f, caroot)
		}
	}
	return caroot
}

// settingsClones flattens the "clone" setting to one entry per repository.
func settingsClones(config vmConfig) []cloneRepo {
	if config.Clone == nil {
		return nil
	}
	var clones []cloneRepo
	for _, g := range *config.Clone {
		if !ghNameRE.MatchString(g.Org) {
			die("settings %s: invalid clone org %q", settingsFile, g.Org)
		}
		if !basedirRE.MatchString(g.Basedir) {
			die("settings %s: invalid clone basedir %q", settingsFile, g.Basedir)
		}
		for _, name := range g.Repositories {
			if !ghNameRE.MatchString(name) {
				die("settings %s: invalid clone repository %q", settingsFile, name)
			}
			clones = append(clones, cloneRepo{basedir: g.Basedir, repo: g.Org + "/" + name})
		}
	}
	return clones
}

// cloneList renders the guest-side list clone-user.sh reads: one
// "<basedir>\t<org>/<repo>" line per repository. The header keeps the file
// non-empty when nothing is configured.
func cloneList(clones []cloneRepo) string {
	list := "# <basedir>\t<org>/<repo>, from the \"clone\" entry of settings.json\n"
	for _, c := range clones {
		list += fmt.Sprintf("%s\t%s\n", c.basedir, c.repo)
	}
	return list
}

// resolveResources: a flag given on the command line wins, then settings.json,
// then the built-in defaults.
func resolveResources(flags resources, set map[string]bool, config vmConfig) resources {
	res := defaultResources
	for _, r := range []struct {
		key     string
		setting *int
		flag    int
		out     *int
	}{
		{"cpus", config.CPUs, flags.cpus, &res.cpus},
		{"memory", config.Memory, flags.memory, &res.memory},
		{"disk", config.Disk, flags.disk, &res.disk},
	} {
		if r.setting != nil {
			if *r.setting <= 0 {
				die("settings %s: %q must be a positive integer", settingsFile, r.key)
			}
			*r.out = *r.setting
		}
		if set[r.key] {
			*r.out = r.flag
		}
	}
	return res
}

// flagsSet lists the flags that appeared on the command line, so an unset flag
// falls back to settings.json instead of its zero value.
func flagsSet(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

func checkResources(res resources) {
	for _, r := range []struct {
		flag string
		n    int
	}{
		{"cpus", res.cpus},
		{"memory", res.memory},
		{"disk", res.disk},
	} {
		if r.n <= 0 {
			die("-%s must be a positive integer, got %d", r.flag, r.n)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func createKey(comment, key, pub string) {
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		die("cannot create %s: %v", keyDir, err)
	}
	if err := os.Chmod(keyDir, 0o700); err != nil {
		die("cannot chmod %s: %v", keyDir, err)
	}
	for _, p := range []string{key, pub} {
		os.Remove(p)
	}
	run("ssh-keygen", "-t", "ed25519", "-N", "", "-C", comment, "-f", key)
	if err := os.Chmod(key, 0o600); err != nil {
		die("cannot chmod %s: %v", key, err)
	}
	fmt.Printf("created key %s\n", key)
}

// registerKey replaces the key recorded in the state file plus any key holding
// the qualified title, then registers the new public key under that title.
func registerKey(name, title, pub string) int64 {
	previous, _ := keyID(getVM(name)["github_key_id"])
	pubData, err := os.ReadFile(pub)
	if err != nil {
		die("cannot read %s: %v", pub, err)
	}
	for _, k := range listKeys() {
		if k.Title == title || k.ID == previous {
			deleteKey(k.ID)
			fmt.Printf("deleted old GitHub key %d (%s)\n", k.ID, k.Title)
		}
	}
	created := addKey(title, strings.TrimSpace(string(pubData)))
	fmt.Printf("registered GitHub key %d (%s)\n", created.ID, title)
	return created.ID
}

// startVM materializes the embedded template tree into a temp directory —
// limactl resolves provision file.url paths relative to the template — drops
// the private key at tmp/default where the template expects it, and boots.
func startVM(name, dotfiles string, res resources, key string, clones []cloneRepo, caroot string) {
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
	if err := os.WriteFile(filepath.Join(dir, "tmp", "clone-list"), []byte(cloneList(clones)), 0o600); err != nil {
		die("cannot materialize template: %v", err)
	}
	// The mkcert CA files are staged the same way, empty when the setting is
	// off: their `mode: data` entries are unconditional, and mkcert-user.sh
	// skips an empty file.
	for _, f := range caFiles {
		var data []byte
		if caroot != "" {
			data, err = os.ReadFile(filepath.Join(caroot, f))
			if err != nil {
				die("cannot read %s: %v", filepath.Join(caroot, f), err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "tmp", f), data, 0o600); err != nil {
			die("cannot materialize template: %v", err)
		}
	}

	limactlRun("start", "--tty=false", "--name", name,
		"--set", startSet(res, dotfiles, caroot),
		filepath.Join(dir, "dev-vm.yaml"))
}

// startSet is the yq expression patching the template at creation time. The
// caCerts entry points at the host CAROOT, not at the temp tree: the hostagent
// re-reads it every time the instance starts, long after that tree is gone.
func startSet(res resources, dotfiles, caroot string) string {
	set := fmt.Sprintf(
		`.cpus = %d | .memory = "%dGiB" | .disk = "%dGiB" | .param.DOTFILES_REPO = %q`,
		res.cpus, res.memory, res.disk, dotfiles)
	if caroot != "" {
		set += fmt.Sprintf(` | .caCerts.files = [%q]`, filepath.Join(caroot, "rootCA.pem"))
	}
	return set
}
