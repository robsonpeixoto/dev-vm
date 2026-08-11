package main

import (
	"flag"
	"fmt"
	"os"
)

const stopUsage = `Stop a running Lima dev VM.

Usage: devvm stop [name] [-force]

- Shuts the VM <name> down gracefully, leaving the disk and the GitHub key
  in place; devvm start brings it back.
- -force kills the VM instead of asking the guest to shut down. Faster, but
  unwritten data in the guest is lost.
- Stopping an already stopped VM is a no-op.
- Fails when there is no such VM; create it with devvm create.

`

func cmdStop(argv []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, stopUsage)
		fs.PrintDefaults()
	}
	var force bool
	fs.BoolVar(&force, "force", false,
		"kill the VM instead of shutting the guest down gracefully")
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
