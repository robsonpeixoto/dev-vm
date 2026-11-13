package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
)

const diskResizeUsage = `Grow the disk of a Lima dev VM.

Usage: devvm disk-resize [name] <GiB>

- Sets the disk of the VM <name> (default "default") to <GiB>, a positive
  integer, with limactl edit --disk. Lima grows a disk, never shrinks it, so
  a size below the current one is refused before anything changes.
- A running VM is stopped for the edit and started again afterwards; a
  stopped VM stays stopped and picks the new size up on its next start.
- Records the size as "disk" under "vms".<name> in
  ~/.config/dev-vm/settings.json, creating the file when missing, so a later
  delete + create keeps it.

`

func cmdDiskResize(argv []string) {
	fs := flag.NewFlagSet("disk-resize", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, diskResizeUsage)
	}
	fs.Parse(argv)
	name, size := "default", ""
	switch fs.NArg() {
	case 1:
		size = fs.Arg(0)
	case 2:
		name, size = fs.Arg(0), fs.Arg(1)
	default:
		fs.Usage()
		die("disk-resize needs [name] <GiB>")
	}
	disk, err := strconv.Atoi(size)
	if err != nil || disk <= 0 {
		die("disk size must be a positive integer in GiB, got %q", size)
	}

	checkName(name)
	// Fail on a broken settings file now, not after the VM was stopped.
	loadSettings(name)
	inst, ok := limaInstances()[name]
	if !ok {
		die("no VM %q to resize; run: devvm create %s", name, name)
	}
	current := inst.Disk >> 30
	if int64(disk) < current {
		die("VM %q has %d GiB of disk; Lima cannot shrink it to %d GiB", name, current, disk)
	}

	if int64(disk) == current {
		setSettingsDisk(name, disk)
		fmt.Printf("disk of %s is already %d GiB\n", name, disk)
		return
	}

	running := inst.Status == "Running"
	if running {
		limactlRun("stop", name)
	}
	limactlRun("edit", "--tty=false", "--disk", strconv.Itoa(disk), name)
	// Record the size before starting: the edit already took, and a failing
	// start (a probe, say) must not leave settings behind it.
	setSettingsDisk(name, disk)
	if running {
		limactlRun("start", "--tty=false", name)
	}
	fmt.Printf("resized %s disk to %d GiB\n", name, disk)
}

// setSettingsDisk writes disk into the "vms".<name> block of settings.json,
// keeping the file's mode and every other key.
func setSettingsDisk(name string, disk int) {
	perm := os.FileMode(0o600)
	data, err := os.ReadFile(settingsFile)
	switch {
	case errors.Is(err, os.ErrNotExist):
		data = []byte("{}")
	case err != nil:
		die("cannot read settings %s: %v", settingsFile, err)
	default:
		if info, err := os.Stat(settingsFile); err == nil {
			perm = info.Mode().Perm()
		}
	}
	out, err := withVMDisk(data, name, disk)
	if err != nil {
		die("cannot update settings %s: %v", settingsFile, err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		die("cannot create %s: %v", stateDir, err)
	}
	tmp := settingsFile + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), perm); err != nil {
		die("cannot write settings %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, settingsFile); err != nil {
		die("cannot write settings %s: %v", settingsFile, err)
	}
}

// withVMDisk returns the settings document with "vms".<name>.disk set. Values
// are carried as raw JSON, so blocks it does not touch keep their key order;
// only the root, "vms" and the edited VM block come out sorted.
func withVMDisk(data []byte, name string, disk int) ([]byte, error) {
	var root map[string]jsontext.Value
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root == nil {
		root = map[string]jsontext.Value{}
	}
	var vms map[string]jsontext.Value
	if raw, ok := root["vms"]; ok {
		if err := json.Unmarshal(raw, &vms); err != nil {
			return nil, fmt.Errorf("vms: %w", err)
		}
	}
	if vms == nil {
		vms = map[string]jsontext.Value{}
	}
	var vm map[string]jsontext.Value
	if raw, ok := vms[name]; ok {
		if err := json.Unmarshal(raw, &vm); err != nil {
			return nil, fmt.Errorf("vms.%s: %w", name, err)
		}
	}
	if vm == nil {
		vm = map[string]jsontext.Value{}
	}

	vm["disk"] = jsontext.Value(strconv.Itoa(disk))
	var err error
	if vms[name], err = json.Marshal(vm, json.Deterministic(true)); err != nil {
		return nil, err
	}
	if root["vms"], err = json.Marshal(vms, json.Deterministic(true)); err != nil {
		return nil, err
	}
	return json.Marshal(root, json.Deterministic(true), jsontext.WithIndent("  "))
}
