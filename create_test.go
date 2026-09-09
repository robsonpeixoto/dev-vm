package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSettingsResources(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		want     resources
	}{
		{
			name: "defaults",
			want: resources{cpus: 2, memory: 2, disk: 50},
		},
		{
			name:     "settings only",
			settings: `{"cpus": 3, "memory": 4, "disk": 55}`,
			want:     resources{cpus: 3, memory: 4, disk: 55},
		},
		{
			name:     "partial settings",
			settings: `{"memory": 8}`,
			want:     resources{cpus: 2, memory: 8, disk: 50},
		},
		{
			name:     "unrelated settings ignored",
			settings: `{"dotfiles": "git@github.com:user/dotfiles.git"}`,
			want:     resources{cpus: 2, memory: 2, disk: 50},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			if got := settingsResources(); got != tc.want {
				t.Errorf("settingsResources() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestResourceFlags drives the same flag wiring as cmdCreate: flags win over
// settings.json, unset flags keep the settings value.
func TestResourceFlags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		argv     []string
		want     resources
	}{
		{
			name: "no flags keeps defaults",
			want: resources{cpus: 2, memory: 2, disk: 50},
		},
		{
			name:     "flag overrides one setting",
			settings: `{"cpus": 3, "memory": 4, "disk": 55}`,
			argv:     []string{"-cpus", "6"},
			want:     resources{cpus: 6, memory: 4, disk: 55},
		},
		{
			name: "all flags",
			argv: []string{"-cpus", "8", "-memory", "16", "-disk", "100"},
			want: resources{cpus: 8, memory: 16, disk: 100},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			fs := flag.NewFlagSet("create", flag.ContinueOnError)
			res := settingsResources()
			fs.IntVar(&res.cpus, "cpus", res.cpus, "")
			fs.IntVar(&res.memory, "memory", res.memory, "")
			fs.IntVar(&res.disk, "disk", res.disk, "")
			if err := fs.Parse(tc.argv); err != nil {
				t.Fatal(err)
			}
			if res != tc.want {
				t.Errorf("resources = %+v, want %+v", res, tc.want)
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
		want     []cloneRepo
	}{
		{
			name: "no settings",
		},
		{
			name:     "unrelated settings",
			settings: `{"cpus": 4}`,
		},
		{
			name: "one group, two repositories",
			settings: `{"clone": [{"org": "robsonpeixoto",
				"basedir": "${HOME}/Code/robsonpeixoto",
				"repositories": ["dev-vm", "echo-server"]}]}`,
			want: []cloneRepo{
				{basedir: "${HOME}/Code/robsonpeixoto", repo: "robsonpeixoto/dev-vm"},
				{basedir: "${HOME}/Code/robsonpeixoto", repo: "robsonpeixoto/echo-server"},
			},
		},
		{
			name: "two groups",
			settings: `{"clone": [
				{"org": "one", "basedir": "/srv/one", "repositories": ["a"]},
				{"org": "two", "basedir": "/srv/two", "repositories": ["b"]}]}`,
			want: []cloneRepo{
				{basedir: "/srv/one", repo: "one/a"},
				{basedir: "/srv/two", repo: "two/b"},
			},
		},
		{
			name:     "group without repositories",
			settings: `{"clone": [{"org": "one", "basedir": "/srv/one", "repositories": []}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			got := settingsClones()
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
