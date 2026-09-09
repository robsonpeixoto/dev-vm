package main

import (
	"encoding/base64"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestResolveResources(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		vm       string
		argv     []string
		want     resources
	}{
		{
			name: "no settings, no flags",
			vm:   "myvm",
			want: resources{cpus: 2, memory: 2, disk: 50},
		},
		{
			name:     "default block",
			settings: `{"default": {"cpus": 3, "memory": 4, "disk": 55}}`,
			vm:       "myvm",
			want:     resources{cpus: 3, memory: 4, disk: 55},
		},
		{
			name:     "partial default block",
			settings: `{"default": {"memory": 8}}`,
			vm:       "myvm",
			want:     resources{cpus: 2, memory: 8, disk: 50},
		},
		{
			name: "vm block overrides the default block",
			settings: `{"default": {"cpus": 8, "memory": 16, "disk": 100},
				"vms": {"myvm": {"cpus": 4, "memory": 4}}}`,
			vm:   "myvm",
			want: resources{cpus: 4, memory: 4, disk: 100},
		},
		{
			name: "vm block applies to its own VM only",
			settings: `{"default": {"cpus": 8, "memory": 16, "disk": 100},
				"vms": {"other": {"cpus": 4}}}`,
			vm:   "myvm",
			want: resources{cpus: 8, memory: 16, disk: 100},
		},
		{
			name:     "flag beats both blocks",
			settings: `{"default": {"cpus": 3}, "vms": {"myvm": {"cpus": 4}}}`,
			vm:       "myvm",
			argv:     []string{"-cpus", "6"},
			want:     resources{cpus: 6, memory: 2, disk: 50},
		},
		{
			name: "all flags",
			vm:   "myvm",
			argv: []string{"-cpus", "8", "-memory", "16", "-disk", "100"},
			want: resources{cpus: 8, memory: 16, disk: 100},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			// Same wiring as cmdCreate: zero flag defaults, settings applied
			// afterwards to whatever the command line left unset.
			fs := flag.NewFlagSet("create", flag.ContinueOnError)
			var flags resources
			fs.IntVar(&flags.cpus, "cpus", 0, "")
			fs.IntVar(&flags.memory, "memory", 0, "")
			fs.IntVar(&flags.disk, "disk", 0, "")
			if err := fs.Parse(tc.argv); err != nil {
				t.Fatal(err)
			}
			got := resolveResources(flags, flagsSet(fs), loadSettings(tc.vm))
			if got != tc.want {
				t.Errorf("resolveResources() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveDotfiles(t *testing.T) {
	for _, tc := range []struct {
		name       string
		settings   string
		vm         string
		flag       string
		noDotfiles bool
		want       string
	}{
		{
			name: "no settings",
			vm:   "myvm",
		},
		{
			name:     "default block",
			settings: `{"default": {"dotfiles": "git@github.com:user/dotfiles.git"}}`,
			vm:       "myvm",
			want:     "git@github.com:user/dotfiles.git",
		},
		{
			name: "vm block overrides the default block",
			settings: `{"default": {"dotfiles": "git@github.com:user/dotfiles.git"},
				"vms": {"myvm": {"dotfiles": "git@github.com:user/other.git"}}}`,
			vm:   "myvm",
			want: "git@github.com:user/other.git",
		},
		{
			name:     "vm block turns dotfiles off",
			settings: `{"default": {"dotfiles": "git@github.com:user/dotfiles.git"}, "vms": {"myvm": {"dotfiles": ""}}}`,
			vm:       "myvm",
		},
		{
			name:     "flag beats settings",
			settings: `{"default": {"dotfiles": "git@github.com:user/dotfiles.git"}}`,
			vm:       "myvm",
			flag:     "git@github.com:user/flag.git",
			want:     "git@github.com:user/flag.git",
		},
		{
			name:       "no-dotfiles beats settings",
			settings:   `{"default": {"dotfiles": "git@github.com:user/dotfiles.git"}}`,
			vm:         "myvm",
			noDotfiles: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			got := resolveDotfiles(tc.flag, tc.noDotfiles, loadSettings(tc.vm))
			if got != tc.want {
				t.Errorf("resolveDotfiles() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResourceFlagRejectsNonInteger(t *testing.T) {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	memory := 2
	fs.IntVar(&memory, "memory", memory, "")
	if err := fs.Parse([]string{"-memory", "8GiB"}); err == nil {
		t.Error("Parse(-memory 8GiB) = nil, want error")
	}
}

// withSettings points settingsFile at a temporary file for the test; empty
// content means no settings file at all.
func withSettings(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := settingsFile
	settingsFile = path
	t.Cleanup(func() { settingsFile = old })
}

func TestSettingsClones(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		vm       string
		want     []cloneRepo
	}{
		{
			name: "no settings",
			vm:   "myvm",
		},
		{
			name:     "settings without a clone key",
			settings: `{"default": {"cpus": 4}}`,
			vm:       "myvm",
		},
		{
			name: "one group, two repositories",
			settings: `{"default": {"clone": [{"org": "robsonpeixoto",
				"basedir": "${HOME}/Code/robsonpeixoto",
				"repositories": ["dev-vm", "echo-server"]}]}}`,
			vm: "myvm",
			want: []cloneRepo{
				{basedir: "${HOME}/Code/robsonpeixoto", repo: "robsonpeixoto/dev-vm"},
				{basedir: "${HOME}/Code/robsonpeixoto", repo: "robsonpeixoto/echo-server"},
			},
		},
		{
			name: "two groups",
			settings: `{"default": {"clone": [
				{"org": "one", "basedir": "/srv/one", "repositories": ["a"]},
				{"org": "two", "basedir": "/srv/two", "repositories": ["b"]}]}}`,
			vm: "myvm",
			want: []cloneRepo{
				{basedir: "/srv/one", repo: "one/a"},
				{basedir: "/srv/two", repo: "two/b"},
			},
		},
		{
			name:     "group without repositories",
			settings: `{"default": {"clone": [{"org": "one", "basedir": "/srv/one", "repositories": []}]}}`,
			vm:       "myvm",
		},
		{
			name: "vm block replaces the default list",
			settings: `{"default": {"clone": [{"org": "one", "basedir": "/srv/one", "repositories": ["a"]}]},
				"vms": {"myvm": {"clone": [{"org": "two", "basedir": "/srv/two", "repositories": ["b"]}]}}}`,
			vm:   "myvm",
			want: []cloneRepo{{basedir: "/srv/two", repo: "two/b"}},
		},
		{
			name: "empty vm list clones nothing",
			settings: `{"default": {"clone": [{"org": "one", "basedir": "/srv/one", "repositories": ["a"]}]},
				"vms": {"myvm": {"clone": []}}}`,
			vm: "myvm",
		},
		{
			name: "vm block keeps the default list for other VMs",
			settings: `{"default": {"clone": [{"org": "one", "basedir": "/srv/one", "repositories": ["a"]}]},
				"vms": {"other": {"clone": []}}}`,
			vm:   "myvm",
			want: []cloneRepo{{basedir: "/srv/one", repo: "one/a"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			got := settingsClones(loadSettings(tc.vm))
			if !slices.Equal(got, tc.want) {
				t.Errorf("settingsClones() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCloneList(t *testing.T) {
	clones := []cloneRepo{
		{basedir: "${HOME}/Code/robsonpeixoto", repo: "robsonpeixoto/dev-vm"},
		{basedir: "/srv/one", repo: "one/a"},
	}
	got := cloneList(clones)
	want := "${HOME}/Code/robsonpeixoto\trobsonpeixoto/dev-vm\n/srv/one\tone/a\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("cloneList() = %q, want it to end with %q", got, want)
	}
	if !strings.HasPrefix(got, "#") {
		t.Errorf("cloneList() = %q, want a comment header keeping the file non-empty", got)
	}
	if empty := cloneList(nil); !strings.HasPrefix(empty, "#") || strings.Count(empty, "\n") != 1 {
		t.Errorf("cloneList(nil) = %q, want only the comment header", empty)
	}
}

func TestMkcertEnabled(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		vm       string
		want     bool
	}{
		{
			name: "no settings",
			vm:   "myvm",
		},
		{
			name:     "default block",
			settings: `{"default": {"mkcert": true}}`,
			vm:       "myvm",
			want:     true,
		},
		{
			name:     "vm block turns mkcert off",
			settings: `{"default": {"mkcert": true}, "vms": {"myvm": {"mkcert": false}}}`,
			vm:       "myvm",
		},
		{
			name:     "vm block turns mkcert on",
			settings: `{"vms": {"myvm": {"mkcert": true}}}`,
			vm:       "myvm",
			want:     true,
		},
		{
			name:     "vm block applies to its own VM only",
			settings: `{"vms": {"other": {"mkcert": true}}}`,
			vm:       "myvm",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			if got := mkcertEnabled(loadSettings(tc.vm)); got != tc.want {
				t.Errorf("mkcertEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGhosttyEnabled(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		vm       string
		want     bool
	}{
		{
			name: "no settings",
			vm:   "myvm",
		},
		{
			name:     "default block",
			settings: `{"default": {"ghostty": true}}`,
			vm:       "myvm",
			want:     true,
		},
		{
			name:     "vm block turns ghostty off",
			settings: `{"default": {"ghostty": true}, "vms": {"myvm": {"ghostty": false}}}`,
			vm:       "myvm",
		},
		{
			name:     "vm block turns ghostty on",
			settings: `{"vms": {"myvm": {"ghostty": true}}}`,
			vm:       "myvm",
			want:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			if got := ghosttyEnabled(loadSettings(tc.vm)); got != tc.want {
				t.Errorf("ghosttyEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTerminfoB64(t *testing.T) {
	if got := terminfoB64(""); got != "" {
		t.Errorf("terminfoB64(\"\") = %q, want the empty string", got)
	}
	// The acsc capability of the real entry carries the braces that make the
	// encoding necessary: Lima templates data content on the host.
	src := "xterm-ghostty|ghostty,\n\tacsc=++\\,\\,--..00``zz{{||}}~~,\n"
	got := terminfoB64(src)
	if strings.Contains(got, "{{") {
		t.Errorf("terminfoB64() = %q, want no Go template braces", got)
	}
	for line := range strings.Lines(got) {
		if n := len(strings.TrimSuffix(line, "\n")); n > 76 {
			t.Errorf("terminfoB64() line %q is %d characters, want at most 76", line, n)
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(got, "\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != src {
		t.Errorf("decoded terminfoB64() = %q, want %q", decoded, src)
	}
}

func TestStartSet(t *testing.T) {
	res := resources{cpus: 4, memory: 8, disk: 100}
	got := startSet(res, "git@github.com:user/dotfiles.git", "", false)
	want := `.cpus = 4 | .memory = "8GiB" | .disk = "100GiB" | .param.DOTFILES_REPO = "git@github.com:user/dotfiles.git"` +
		` | .nestedVirtualization = false`
	if got != want {
		t.Errorf("startSet() = %q, want %q", got, want)
	}
	// The CA path is the host one, quoted: the default CAROOT on macOS has a
	// space in it.
	got = startSet(res, "", "/Users/x/Library/Application Support/mkcert", false)
	want += ` | .caCerts.files = ["/Users/x/Library/Application Support/mkcert/rootCA.pem"]`
	want = strings.Replace(want, `"git@github.com:user/dotfiles.git"`, `""`, 1)
	if got != want {
		t.Errorf("startSet() with a CAROOT = %q, want %q", got, want)
	}
	got = startSet(res, "", "", true)
	if !strings.Contains(got, ".nestedVirtualization = true") {
		t.Errorf("startSet() with nesting = %q, want .nestedVirtualization = true", got)
	}
}

func TestResolveNested(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		vm       string
		argv     []string
		want     bool
	}{
		{
			name: "no settings, no flag",
			vm:   "myvm",
		},
		{
			name:     "default block",
			settings: `{"default": {"nested": true}}`,
			vm:       "myvm",
			want:     true,
		},
		{
			name:     "vm block turns nesting off",
			settings: `{"default": {"nested": true}, "vms": {"myvm": {"nested": false}}}`,
			vm:       "myvm",
		},
		{
			name:     "vm block applies to its own VM only",
			settings: `{"vms": {"other": {"nested": true}}}`,
			vm:       "myvm",
		},
		{
			name: "flag alone",
			vm:   "myvm",
			argv: []string{"-nested"},
			want: true,
		},
		{
			name:     "flag beats the settings",
			settings: `{"default": {"nested": true}}`,
			vm:       "myvm",
			argv:     []string{"-nested=false"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			fs := flag.NewFlagSet("create", flag.ContinueOnError)
			var nested bool
			fs.BoolVar(&nested, "nested", false, "")
			if err := fs.Parse(tc.argv); err != nil {
				t.Fatal(err)
			}
			if got := resolveNested(nested, flagsSet(fs), loadSettings(tc.vm)); got != tc.want {
				t.Errorf("resolveNested() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNestedSupported(t *testing.T) {
	for _, tc := range []struct {
		brand string
		want  bool
	}{
		{brand: "Apple M1", want: false},
		{brand: "Apple M2 Max", want: false},
		{brand: "Apple M3", want: true},
		{brand: "Apple M4 Pro", want: true},
		{brand: "Apple M5 Pro", want: true},
		{brand: "Apple M10", want: true},
		{brand: "Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz", want: false},
		{brand: "", want: false},
	} {
		t.Run(tc.brand, func(t *testing.T) {
			if got := nestedSupported(tc.brand); got != tc.want {
				t.Errorf("nestedSupported(%q) = %v, want %v", tc.brand, got, tc.want)
			}
		})
	}
}
