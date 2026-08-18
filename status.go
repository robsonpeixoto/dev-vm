package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
)

const statusUsage = `Show one dev VM in detail.

Usage: devvm status [name]

- Same data as devvm list for a single VM (name defaults to "default"), plus
  what the state file records: dotfiles repo, key paths and creation time.
- IP comes from the guest, so a VM that is not running shows "-". Reach guest
  services at that IP; there are no port forwards.
- A name with no Lima instance shows status Missing instead of failing.

`

func cmdStatus(argv []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, statusUsage)
	}
	name := parseArgs(fs, argv)

	checkName(name)
	inst, exists := limaInstances()[name]
	entry := getVM(name)

	status, cpus, mem, disk, ssh := "Missing", "-", "-", "-", "-"
	ip := ""
	if exists {
		status = inst.Status
		if inst.CPUs > 0 {
			cpus = fmt.Sprint(inst.CPUs)
		}
		mem, disk = gib(inst.Memory), gib(inst.Disk)
		ssh = fmt.Sprintf("ssh -F %s %s",
			filepath.Join(homeDir(), ".lima", name, "ssh.config"), inst.Hostname)
		if inst.Status == "Running" {
			ip = guestIP(inst)
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, f := range []struct{ label, value string }{
		{"NAME", name},
		{"STATUS", status},
		{"CPUS", cpus},
		{"MEM", mem},
		{"DISK", disk},
		{"IP", dash(ip)},
		{"SSH", ssh},
		{"DOTFILES", dash(stateString(entry, "dotfiles"))},
		{"KEY", dash(stateString(entry, "private_key"))},
		{"CREATED", dash(stateString(entry, "created_at"))},
	} {
		fmt.Fprintf(w, "%s\t%s\n", f.label, f.value)
	}
	w.Flush()
}

// stateString reads a string field out of a state entry, empty when absent.
func stateString(entry map[string]any, key string) string {
	s, _ := entry[key].(string)
	return s
}
