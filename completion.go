package main

import (
	"embed"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
)

//go:embed completions
var completionScripts embed.FS

const completionUsage = `Print the shell completion script for dev-vm.

Usage: devvm completion <bash|zsh|fish>

Load it for the current shell:

  bash: source <(devvm completion bash)
  zsh:  source <(devvm completion zsh)
  fish: devvm completion fish | source

Install it permanently:

  bash: devvm completion bash > /usr/local/etc/bash_completion.d/dev-vm
  zsh:  devvm completion zsh > "${fpath[1]}/_dev-vm"
  fish: devvm completion fish > ~/.config/fish/completions/dev-vm.fish

The script completes VM names for destroy by calling "devvm __names".

`

func cmdCompletion(argv []string) {
	fs := flag.NewFlagSet("completion", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, completionUsage)
	}
	fs.Parse(argv)
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, completionUsage)
		die("completion needs exactly one shell: bash, zsh or fish")
	}

	shell := fs.Arg(0)
	script, err := completionScripts.ReadFile("completions/dev-vm." + shell)
	if err != nil {
		die("unsupported shell %q; use bash, zsh or fish", shell)
	}
	os.Stdout.Write(script)
}

// cmdNames prints the recorded VM names, one per line, for the completion
// scripts.
func cmdNames() {
	vms, _ := loadState()["vms"].(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(vms)) {
		fmt.Println(name)
	}
}
