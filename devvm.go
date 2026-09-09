// Shared helpers for the create and delete subcommands.
//
// All external work goes through command line tools: limactl and gh.
// VM metadata lives in a JSON state file under ~/.config/dev-vm, alongside an
// optional user-written settings.json holding a "default" block — the dotfiles
// repo, the VM size, the repositories to clone, the mkcert CA — and per-VM
// overrides of it under "vms". SSH key pairs are kept in ~/.config/dev-vm/keys.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	stateVersion = 1
	scopeHint    = "run: gh auth refresh -h github.com -s admin:public_key"
)

var (
	stateDir     = filepath.Join(homeDir(), ".config", "dev-vm")
	stateFile    = filepath.Join(stateDir, "state.json")
	settingsFile = filepath.Join(stateDir, "settings.json")
	keyDir       = filepath.Join(stateDir, "keys")

	nameRE = regexp.MustCompile(`^[A-Za-z0-9]+(?:[._-][A-Za-z0-9]+)*$`)
	repoRE = regexp.MustCompile(`^[A-Za-z0-9@:._/+~-]+$`)
	// GitHub org and repository names, for the "clone" setting.
	ghNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// Clone target directories: absolute or $HOME-relative paths, no spaces
	// and no shell metacharacters beyond the ${HOME} the guest expands.
	basedirRE = regexp.MustCompile(`^[A-Za-z0-9${}/._~-]+$`)
)

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		die("cannot determine home directory: %v", err)
	}
	return home
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func checkName(name string) {
	if !nameRE.MatchString(name) {
		die("invalid VM name %q", name)
	}
}

// checkRepo guards the repo URL: it is interpolated into a yq expression and
// a shell variable.
func checkRepo(repo string) {
	if !repoRE.MatchString(repo) {
		die("invalid dotfiles repo %q", repo)
	}
}

func keyPaths(name string) (key, pub string) {
	return filepath.Join(keyDir, name), filepath.Join(keyDir, name+".pub")
}

// keyTitle qualifies the GitHub key title with the host machine so two
// machines creating the same VM name do not delete each other's key.
func keyTitle(name string) string {
	return fmt.Sprintf("dev-vm/%s/%s", hostName(), name)
}

// hostName is the short host name, without the .local mDNS suffix macOS adds.
func hostName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "unknown-host"
	}
	host = strings.TrimSuffix(host, ".local")
	if short, _, ok := strings.Cut(host, "."); ok {
		host = short
	}
	return host
}

// vmConfig is one settings block: the "default" object, or a "vms".<name>
// object layered over it. The pointers separate an absent key from an explicit
// override such as "clone": [].
type vmConfig struct {
	CPUs     *int          `json:"cpus"`
	Memory   *int          `json:"memory"`
	Disk     *int          `json:"disk"`
	Dotfiles *string       `json:"dotfiles"`
	Clone    *[]cloneGroup `json:"clone"`
	Mkcert   *bool         `json:"mkcert"`
}

// cloneGroup is one "clone" entry: repositories of a single GitHub org, all
// cloned under basedir in the guest.
type cloneGroup struct {
	Org          string   `json:"org"`
	Basedir      string   `json:"basedir"`
	Repositories []string `json:"repositories"`
}

// loadSettings reads ~/.config/dev-vm/settings.json and returns the config for
// one VM: the "default" block with the matching "vms" block layered on top,
// e.g. {"default": {"cpus": 4}, "vms": {"big": {"cpus": 16}}}. Unknown keys are
// an error, which is also what an old flat settings file hits.
func loadSettings(name string) vmConfig {
	data, err := os.ReadFile(settingsFile)
	if errors.Is(err, os.ErrNotExist) {
		return vmConfig{}
	}
	if err != nil {
		die("cannot read settings %s: %v", settingsFile, err)
	}
	var file struct {
		Default vmConfig            `json:"default"`
		VMs     map[string]vmConfig `json:"vms"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		hint := ""
		if strings.Contains(err.Error(), "unknown field") {
			hint = "; every VM key belongs under \"default\" or \"vms\".<name>"
		}
		die("cannot read settings %s: %v%s", settingsFile, err, hint)
	}
	config := file.Default
	if override, ok := file.VMs[name]; ok {
		config = mergeConfig(config, override)
	}
	return config
}

// mergeConfig layers a per-VM block over the default one, key by key.
func mergeConfig(base, over vmConfig) vmConfig {
	if over.CPUs != nil {
		base.CPUs = over.CPUs
	}
	if over.Memory != nil {
		base.Memory = over.Memory
	}
	if over.Disk != nil {
		base.Disk = over.Disk
	}
	if over.Dotfiles != nil {
		base.Dotfiles = over.Dotfiles
	}
	if over.Clone != nil {
		base.Clone = over.Clone
	}
	if over.Mkcert != nil {
		base.Mkcert = over.Mkcert
	}
	return base
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		die("%s failed: %s", name, strings.TrimSpace(stderr.String()))
	}
}

func limactl(args ...string) string {
	cmd := exec.Command("limactl", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	limactlCheck(err, stderr.String(), args)
	return string(out)
}

// limactlTry runs limactl under a deadline and returns the error instead of
// exiting, for callers that must survive a stopped or wedged VM.
func limactlTry(timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, "limactl", args...).Output()
}

func limactlRun(args ...string) {
	cmd := exec.Command("limactl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	limactlCheck(cmd.Run(), "", args)
}

func limactlCheck(err error, stderr string, args []string) {
	if err == nil {
		return
	}
	if errors.Is(err, exec.ErrNotFound) {
		die("limactl not found; install Lima")
	}
	die("limactl %s failed: %s", strings.Join(args, " "), strings.TrimSpace(stderr))
}

func vmExists(name string) bool {
	for _, vm := range strings.Fields(limactl("list", "--quiet")) {
		if vm == name {
			return true
		}
	}
	return false
}

// requireVM stops the caller when Lima has no instance by that name.
func requireVM(name, verb string) {
	if !vmExists(name) {
		die("no VM %q to %s; run: devvm create %s", name, verb, name)
	}
}

func gh(args ...string) string {
	cmd := exec.Command("gh", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			die("gh not found; install GitHub CLI")
		}
		msg := strings.TrimSpace(stderr.String())
		hint := ""
		if strings.Contains(msg, "HTTP 403") || strings.Contains(msg, "HTTP 404") {
			hint = "\n" + scopeHint
		}
		die("gh %s failed: %s%s", strings.Join(args, " "), msg, hint)
	}
	return string(out)
}

func checkScopes() {
	for line := range strings.Lines(gh("api", "-i", "user")) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		value, ok := strings.CutPrefix(strings.ToLower(line), "x-oauth-scopes:")
		if !ok {
			continue
		}
		scopes := strings.Split(value, ",")
		if len(scopes) == 1 && strings.TrimSpace(scopes[0]) == "" {
			return
		}
		for _, s := range scopes {
			if strings.TrimSpace(s) == "admin:public_key" {
				return
			}
		}
		die("token lacks admin:public_key scope; %s", scopeHint)
	}
}

type ghKey struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// listKeys fetches all SSH keys on the account. `gh api --paginate` may emit
// one JSON array per page back to back, so decode until the stream runs dry.
func listKeys() []ghKey {
	dec := json.NewDecoder(strings.NewReader(gh("api", "--paginate", "user/keys")))
	var keys []ghKey
	for dec.More() {
		var page []ghKey
		if err := dec.Decode(&page); err != nil {
			die("cannot parse gh api user/keys output: %v", err)
		}
		keys = append(keys, page...)
	}
	return keys
}

func addKey(title, pub string) ghKey {
	out := gh("api", "--method", "POST", "user/keys",
		"-f", "title="+title, "-f", "key="+pub)
	var created ghKey
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		die("cannot parse gh api user/keys response: %v", err)
	}
	return created
}

func deleteKey(id int64) {
	gh("api", "--method", "DELETE", fmt.Sprintf("user/keys/%d", id), "--silent")
}

func loadState() map[string]any {
	state := map[string]any{}
	data, err := os.ReadFile(stateFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		die("cannot read state %s: %v", stateFile, err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			die("cannot read state %s: %v", stateFile, err)
		}
	}
	if _, ok := state["version"]; !ok {
		state["version"] = stateVersion
	}
	if _, ok := state["vms"].(map[string]any); !ok {
		state["vms"] = map[string]any{}
	}
	return state
}

func saveState(state map[string]any) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		die("cannot create %s: %v", stateDir, err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		die("cannot encode state: %v", err)
	}
	tmp := stateFile + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		die("cannot write state %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, stateFile); err != nil {
		die("cannot write state %s: %v", stateFile, err)
	}
}

func getVM(name string) map[string]any {
	entry, _ := loadState()["vms"].(map[string]any)[name].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	return entry
}

func putVM(name string, fields map[string]any) {
	state := loadState()
	vms := state["vms"].(map[string]any)
	entry, _ := vms[name].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
		vms[name] = entry
	}
	for k, v := range fields {
		entry[k] = v
	}
	entry["name"] = name
	if _, ok := entry["created_at"]; !ok {
		entry["created_at"] = now()
	}
	entry["updated_at"] = now()
	saveState(state)
}

func dropVM(name string) bool {
	state := loadState()
	vms := state["vms"].(map[string]any)
	if _, ok := vms[name]; !ok {
		return false
	}
	delete(vms, name)
	saveState(state)
	return true
}

// keyID reads a GitHub key id out of a state entry, where JSON decoding has
// turned it into a float64.
func keyID(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}
