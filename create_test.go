package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIntFlagSet(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{
		{in: "4", want: 4, ok: true},
		{in: "1", want: 1, ok: true},
		{in: "0"},
		{in: "-1"},
		{in: "8GiB"},
		{in: ""},
	} {
		var f intFlag
		err := f.Set(tc.in)
		if tc.ok {
			if err != nil {
				t.Errorf("Set(%q) = %v, want nil", tc.in, err)
				continue
			}
			if !f.set || f.value != tc.want {
				t.Errorf("Set(%q) gave %+v, want value %d", tc.in, f, tc.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("Set(%q) = nil, want error", tc.in)
		}
	}
}

func TestResolveResources(t *testing.T) {
	for _, tc := range []struct {
		name            string
		settings        string
		cpus, mem, disk intFlag
		want            resources
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
			name:     "flag overrides one setting",
			settings: `{"cpus": 3, "memory": 4, "disk": 55}`,
			cpus:     intFlag{set: true, value: 6},
			want:     resources{cpus: 6, memory: 4, disk: 55},
		},
		{
			name: "flags only",
			cpus: intFlag{set: true, value: 8},
			mem:  intFlag{set: true, value: 16},
			disk: intFlag{set: true, value: 100},
			want: resources{cpus: 8, memory: 16, disk: 100},
		},
		{
			name:     "unrelated settings ignored",
			settings: `{"dotfiles": "git@github.com:user/dotfiles.git"}`,
			want:     resources{cpus: 2, memory: 2, disk: 50},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, tc.settings)
			if got := resolveResources(tc.cpus, tc.mem, tc.disk); got != tc.want {
				t.Errorf("resolveResources() = %+v, want %+v", got, tc.want)
			}
		})
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
