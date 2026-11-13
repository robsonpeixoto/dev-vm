package main

import (
	"flag"
	"fmt"
	"os"
)

const stopUsage = `Stop a running Lima dev VM.

Usage: devvm stop [name] [-force]

- Stops the VM <name> with a clean guest shutdown. The disk and the GitHub key
  stay in place. devvm start starts the VM again.
- -force kills the VM and does not ask the guest for a shutdown. This is
  faster, but the guest loses data that is not yet on its disk.
- If the VM is already stopped, the command does nothing.
- If the VM does not exist, the command fails. Use devvm create to create it.

`

func cmdStop(argv []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, stopUsage)
		fs.PrintDefaults()
	}
	var force bool
	fs.BoolVar(&force, "force", false,
		"kill the VM instead of a clean guest shutdown")
	name := parseArgs(fs, argv)

	checkName(name)
	requireVM(name, "stop")
	args := []string{"stop"}
	if force {
		args = append(args, "-f")
	}
	limactlRun(append(args, name)...)
	fmt.Printf("stopped %s\n", name)
}
