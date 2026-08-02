// devvm manages isolated Lima dev VMs with SSH access to GitHub.
package main

import (
	"fmt"
	"os"
)

const usage = `Manage isolated Lima dev VMs with SSH access to GitHub.

Usage:
  devvm create [name] [--create-ssh-key[=no]] [--dotfiles[=REPO]|--no-dotfiles]
  devvm destroy [name]

Run a command with --help for details.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "create":
		cmdCreate(os.Args[2:])
	case "destroy":
		cmdDestroy(os.Args[2:])
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		die("unknown command %q", os.Args[1])
	}
}
