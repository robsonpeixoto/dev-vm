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

func TestCompletionRejectsUnknownShell(t *testing.T) {
	if _, err := completionScripts.ReadFile("completions/dev-vm.powershell"); err == nil {
		t.Error("ReadFile(dev-vm.powershell) = nil error, want error")
	}
}
