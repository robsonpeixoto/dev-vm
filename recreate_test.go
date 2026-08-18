package main

import (
	"io"
	"slices"
	"strings"
	"testing"
)

func TestConfirmRecreate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{name: "y", answer: "y\n"},
		{name: "yes", answer: "yes\n"},
		{name: "uppercase", answer: "  Y  \n"},
		{name: "no newline", answer: "y"},
		{name: "n", answer: "n\n", wantErr: true},
		{name: "name typed back", answer: "myvm\n", wantErr: true},
		{name: "empty", answer: "\n", wantErr: true},
		{name: "eof", answer: "", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := confirmRecreate("myvm", strings.NewReader(tc.answer), io.Discard)
			if (err != nil) != tc.wantErr {
				t.Errorf("confirmRecreate(%q) = %v, wantErr %v", tc.answer, err, tc.wantErr)
			}
		})
	}
}

// TestRecreateResources: an unset flag keeps the size Lima reports for the
// instance, a passed flag wins, and a size Lima does not report falls back to
// the create defaults.
func TestRecreateResources(t *testing.T) {
	full := limaInstance{CPUs: 8, Memory: 16 << 30, Disk: 100 << 30}
	for _, tc := range []struct {
		name  string
		flags resources
		inst  limaInstance
		want  resources
	}{
		{
			name: "no flags keeps the instance size",
			inst: full,
			want: resources{cpus: 8, memory: 16, disk: 100},
		},
		{
			name:  "one flag wins",
			flags: resources{memory: 4},
			inst:  full,
			want:  resources{cpus: 8, memory: 4, disk: 100},
		},
		{
			name:  "all flags win",
			flags: resources{cpus: 1, memory: 2, disk: 3},
			inst:  full,
			want:  resources{cpus: 1, memory: 2, disk: 3},
		},
		{
			name: "unreported size falls back to defaults",
			inst: limaInstance{},
			want: defaultResources,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, "")
			if got := recreateResources(tc.flags, tc.inst); got != tc.want {
				t.Errorf("recreateResources() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestArchiveCommand: the guest tar must skip everything provisioning owns,
// or the rebuilt VM gets the old ssh key back and loses GitHub access.
func TestArchiveCommand(t *testing.T) {
	cmd := archiveCommand()
	for _, path := range homeExcludes {
		if !strings.Contains(cmd, "--exclude="+path) {
			t.Errorf("archiveCommand() does not exclude %q: %s", path, cmd)
		}
	}
	for _, want := range []string{`tar -C "$HOME"`, "-czf -"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("archiveCommand() = %q, want it to contain %q", cmd, want)
		}
	}
	if !strings.HasSuffix(cmd, " .") {
		t.Errorf("archiveCommand() = %q, want it to archive the whole home", cmd)
	}
}

func TestHomeExcludesCoverProvisionedSSHFiles(t *testing.T) {
	for _, path := range []string{
		"./.ssh/id_ed25519",
		"./.ssh/config",
		"./.ssh/lima-github.conf",
		"./.ssh/authorized_keys",
	} {
		if !slices.Contains(homeExcludes, path) {
			t.Errorf("homeExcludes is missing %q", path)
		}
	}
}
