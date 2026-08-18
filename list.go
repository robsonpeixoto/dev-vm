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
	"sync"
	"text/tabwriter"
	"time"
)

const listUsage = `List dev VMs with their IP and SSH hostname.

Usage: devvm list

- Shows every VM recorded in ~/.config/dev-vm/state.json with its Lima
  status, its guest IP and the SSH hostname from Lima's generated ssh config.
- CPUS, MEM and DISK come from Lima, so they show the instance's real size
  rather than the flags it was created with.
- IP is read from the guest itself, the only place it exists: Lima does not
  report it. A VM that is not running, or that does not answer in time, shows
  "-".
- Reach a service in the guest at that IP; there are no port forwards.
- Connect with: ssh -F ~/.lima/<name>/ssh.config <hostname>
- VMs in the state file without a Lima instance show status Missing.

`

// guestIPTimeout bounds the single guest query per VM, so a wedged guest
// cannot hang list or status.
const guestIPTimeout = 5 * time.Second

type limaInstance struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
	CPUs     int    `json:"cpus"`
	Memory   int64  `json:"memory"` // bytes
	Disk     int64  `json:"disk"`   // bytes
	Network  []struct {
		Interface string `json:"interface"`
	} `json:"network"`
}

// iface is the guest network interface Lima assigns to the VM's own IP.
func (inst limaInstance) iface() string {
	for _, n := range inst.Network {
		if n.Interface != "" {
			return n.Interface
		}
	}
	return "lima0"
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

	names := slices.Sorted(maps.Keys(vms))
	instances := limaInstances()
	ips := guestIPs(instances, names)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tCPUS\tMEM\tDISK\tIP\tSSH")
	for _, name := range names {
		status, host := "Missing", "-"
		cpus, mem, disk := "-", "-", "-"
		if inst, ok := instances[name]; ok {
			status, host = inst.Status, inst.Hostname
			if inst.CPUs > 0 {
				cpus = strconv.Itoa(inst.CPUs)
			}
			mem, disk = gib(inst.Memory), gib(inst.Disk)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			name, status, cpus, mem, disk, dash(ips[name]), host)
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

// dash renders an empty value as "-", like the size columns.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
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

// guestIPs asks every running VM for its own IP, one attempt each and all in
// parallel. Names without a running instance, and queries that fail, are left
// out of the map.
func guestIPs(instances map[string]limaInstance, names []string) map[string]string {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		ips = map[string]string{}
	)
	for _, name := range names {
		inst, ok := instances[name]
		if !ok || inst.Status != "Running" {
			continue
		}
		wg.Go(func() {
			ip := guestIP(inst)
			if ip == "" {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			ips[name] = ip
		})
	}
	wg.Wait()
	return ips
}

// guestIP reads the VM's own IP from inside the guest. Lima reports no guest
// IP: `limactl list --format json` carries only the MAC, and the macOS DHCP
// lease file keeps stale rows, so the guest is the only source of truth.
func guestIP(inst limaInstance) string {
	out, err := limactlTry(guestIPTimeout,
		"shell", inst.Name, "ip", "-4", "-json", "addr", "show", inst.iface())
	if err != nil {
		return ""
	}
	return parseGuestIP(out)
}

// parseGuestIP picks the first IPv4 address out of `ip -4 -json addr` output,
// or returns "" when there is none.
func parseGuestIP(out []byte) string {
	var links []struct {
		AddrInfo []struct {
			Family string `json:"family"`
			Local  string `json:"local"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(out, &links); err != nil {
		return ""
	}
	for _, link := range links {
		for _, addr := range link.AddrInfo {
			if addr.Family == "inet" && addr.Local != "" {
				return addr.Local
			}
		}
	}
	return ""
}
