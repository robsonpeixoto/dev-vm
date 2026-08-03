package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
)

const listUsage = `List dev VMs with their SSH hostname.

Usage: devvm list

- Shows every VM recorded in ~/.config/dev-vm/state.json with its Lima
  status and the SSH hostname from Lima's generated ssh config.
- CPUS, MEM and DISK come from Lima, so they show the instance's real size
  rather than the flags it was created with.
- Connect with: ssh -F ~/.lima/<name>/ssh.config <hostname>
- VMs in the state file without a Lima instance show status Missing.

`

type limaInstance struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
	CPUs     int    `json:"cpus"`
	Memory   int64  `json:"memory"` // bytes
	Disk     int64  `json:"disk"`   // bytes
}

func cmdList(argv []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, listUsage)
	}
	fs.Parse(argv)
	if fs.NArg() > 0 {
		die("unexpected argument %q", fs.Arg(0))
	}

	vms, _ := loadState()["vms"].(map[string]any)
	if len(vms) == 0 {
		fmt.Println("no VMs recorded; run: devvm create")
		return
	}

	instances := limaInstances()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tCPUS\tMEM\tDISK\tSSH")
	for _, name := range slices.Sorted(maps.Keys(vms)) {
		status, host := "Missing", "-"
		cpus, mem, disk := "-", "-", "-"
		if inst, ok := instances[name]; ok {
			status, host = inst.Status, inst.Hostname
			if inst.CPUs > 0 {
				cpus = strconv.Itoa(inst.CPUs)
			}
			mem, disk = gib(inst.Memory), gib(inst.Disk)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", name, status, cpus, mem, disk, host)
	}
	w.Flush()
}

// gib renders a Lima byte count as GiB, or "-" when unknown.
func gib(n int64) string {
	if n <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dGiB", n>>30)
}

// limaInstances indexes `limactl list --format json` output, one JSON object
// per line, by instance name.
func limaInstances() map[string]limaInstance {
	dec := json.NewDecoder(strings.NewReader(limactl("list", "--format", "json")))
	instances := map[string]limaInstance{}
	for dec.More() {
		var inst limaInstance
		if err := dec.Decode(&inst); err != nil {
			die("cannot parse limactl list output: %v", err)
		}
		instances[inst.Name] = inst
	}
	return instances
}
