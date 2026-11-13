package main

import (
	"flag"
	"fmt"
	"os"
)

const startUsage = `Start a stopped Lima dev VM.

Usage: devvm start [name]

- Boots the VM <name>. Lima runs every provisioning step and the readiness
  probes again. Thus the command returns when the VM is usable.
- If the VM is already running, the command does nothing.
- If the VM does not exist, the command fails. Use devvm create to create it.

`

func cmdStart(argv []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, startUsage)
		fs.PrintDefaults()
	}
	name := parseArgs(fs, argv)

	checkName(name)
	requireVM(name, "start")
	limactlRun("start", "--tty=false", name)
	fmt.Printf("started %s\n", name)
}
