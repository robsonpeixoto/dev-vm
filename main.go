// dev-vm manages isolated Lima dev VMs with SSH access to GitHub.
package main

import (
	"fmt"
	"os"
)

// version is set at build time with -X main.version=<tag>.
var version = "dev"

const usage = `Manage isolated Lima dev VMs with SSH access to GitHub.

Usage:
  dev-vm create [name] [--create-ssh-key[=no]] [--dotfiles[=REPO]|--no-dotfiles]
                [-cpus N] [-memory GiB] [-disk GiB]
  dev-vm destroy [name]
  dev-vm list
  dev-vm version

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
	case "list":
		cmdList(os.Args[2:])
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		die("unknown command %q", os.Args[1])
	}
}
