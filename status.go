package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
)

const statusUsage = `Show one dev VM in detail.

Usage: devvm status [name] [-ip]

- Shows the same data as devvm list for one VM. The default name is
  "default". It also shows the dotfiles repo, the key paths and the creation
  time from the state file.
- IP comes from the guest. If the VM is not running, IP shows "-". There are no
  port forwards. Connect to guest services at that IP.
- -ip prints only the guest IP, with no label, for use in scripts:
  curl "http://$(devvm status -ip):8000". If the VM is not running or does not
  answer in the timeout, the command prints an empty line and exits 0.
- If a name has no Lima instance, the status is Missing and the command does
  not fail.

`

func cmdStatus(argv []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, statusUsage)
	}
	ipOnly := fs.Bool("ip", false, "print only the guest IP, empty when unavailable")
	name := parseArgs(fs, argv)

	checkName(name)
	inst, exists := limaInstances()[name]

	if *ipOnly {
		ip := ""
		if exists && inst.Status == "Running" {
			ip = guestIP(inst)
		}
		fmt.Println(ip)
		return
	}

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
