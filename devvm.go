// Shared helpers for the create, recreate and destroy subcommands.
//
// All external work goes through command line tools: limactl and gh.
// VM metadata lives in a JSON state file under ~/.config/dev-vm, alongside an
// optional user-written settings.json holding defaults such as the dotfiles
// repo and the VM size. SSH key pairs are kept in ~/.config/dev-vm/keys.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	backupDir    = filepath.Join(stateDir, "backups")

	nameRE = regexp.MustCompile(`^[A-Za-z0-9]+(?:[._-][A-Za-z0-9]+)*$`)
	repoRE = regexp.MustCompile(`^[A-Za-z0-9@:._/+~-]+$`)
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

// stamp is a filename-safe UTC timestamp for backup archives.
func stamp() string {
	return time.Now().UTC().Format("20060102T150405Z")
}

// mib renders a byte count as MiB, for progress messages about archives.
func mib(n int64) string {
	return fmt.Sprintf("%.1fMiB", float64(n)/(1<<20))
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

// loadSettings reads user defaults from ~/.config/dev-vm/settings.json,
// e.g. {"dotfiles": "<repo>", "cpus": 4, "memory": 8, "disk": 100}.
func loadSettings() map[string]any {
	data, err := os.ReadFile(settingsFile)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}
	}
	if err != nil {
		die("cannot read settings %s: %v", settingsFile, err)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		die("cannot read settings %s: %v", settingsFile, err)
	}
	settings, ok := raw.(map[string]any)
	if !ok {
		die("settings %s must be a JSON object", settingsFile)
	}
	return settings
}

// settingsInt reads a positive integer setting; JSON decoding gives float64.
func settingsInt(key string, v any) int {
	n, ok := v.(float64)
	if !ok || n != float64(int(n)) || n <= 0 {
		die("settings %s: %q must be a positive integer", settingsFile, key)
	}
	return int(n)
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

// limactlPipe runs limactl with stdin and stdout wired to the caller, for
// streaming a tar archive out of or into the guest, and returns the error
// instead of exiting: a non-zero tar exit needs interpreting, not dying.
func limactlPipe(stdin io.Reader, stdout io.Writer, args ...string) error {
	cmd := exec.Command("limactl", args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		die("limactl not found; install Lima")
	}
	return err
}

// exitCode is the process exit code, or -1 when the command never ran.
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
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
