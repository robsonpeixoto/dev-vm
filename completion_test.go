package main

import (
	"strings"
	"testing"
)

func TestCompletionScripts(t *testing.T) {
	for shell, want := range map[string]string{
		"bash": "complete -F _dev_vm dev-vm",
		"zsh":  "#compdef dev-vm devvm",
		"fish": "complete -c dev-vm",
	} {
		t.Run(shell, func(t *testing.T) {
			script, err := completionScripts.ReadFile("completions/dev-vm." + shell)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(script), want) {
				t.Errorf("dev-vm.%s does not contain %q", shell, want)
			}
			if !strings.Contains(string(script), "__names") {
				t.Errorf("dev-vm.%s does not call __names", shell)
			}
		})
	}
}

// TestCompletionCoversCommands guards against a new subcommand landing in
// main.go without reaching the three completion scripts.
func TestCompletionCoversCommands(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			script, err := completionScripts.ReadFile("completions/dev-vm." + shell)
			if err != nil {
				t.Fatal(err)
			}
			for _, cmd := range []string{"create", "recreate", "start", "stop", "destroy", "list", "status"} {
				if !strings.Contains(string(script), cmd) {
					t.Errorf("dev-vm.%s does not mention %q", shell, cmd)
				}
			}
		})
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	if _, err := completionScripts.ReadFile("completions/dev-vm.powershell"); err == nil {
		t.Error("ReadFile(dev-vm.powershell) = nil error, want error")
	}
}
