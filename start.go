package main

import (
	"flag"
	"fmt"
	"os"
)

const startUsage = `Start a stopped Lima dev VM.

Usage: devvm start [name]

- Boots the VM <name>. Lima re-runs every provisioning step and the
  readiness probes, so the command returns once the VM is usable.
- Starting an already running VM is a no-op.
- Fails when there is no such VM; create it with devvm create.

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
