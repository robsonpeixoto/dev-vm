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
  dev-vm create [name] [-create-ssh-key=false] [-dotfiles REPO|-no-dotfiles]
                [-cpus N] [-memory GiB] [-disk GiB]
  dev-vm start [name]
  dev-vm stop [name] [-force]
  dev-vm destroy [name] [-force]
  dev-vm list
  dev-vm completion <bash|zsh|fish>
  dev-vm version

Run a command with -help for details.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "create":
		cmdCreate(os.Args[2:])
	case "start":
		cmdStart(os.Args[2:])
	case "stop":
		cmdStop(os.Args[2:])
	case "destroy":
		cmdDestroy(os.Args[2:])
	case "list":
		cmdList(os.Args[2:])
	case "completion":
		cmdCompletion(os.Args[2:])
	case "__names":
		cmdNames()
	case "version":
		fmt.Println(version)
	case "help", "-h", "-help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		die("unknown command %q", os.Args[1])
	}
}
