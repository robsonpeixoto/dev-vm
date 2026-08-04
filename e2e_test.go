//go:build e2e

// End-to-end lifecycle test: create a real Lima VM, check what provisioning
// installed, restart it, destroy it. Behind the e2e build tag because it boots
// a VM and takes tens of minutes:
//
//	go test -tags e2e -timeout 90m -v -run TestVMLifecycle .
//
// The VM is created with -github-key=false, so the test needs neither gh nor a
// GitHub token; the GitHub SSH readiness probe is skipped with it.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVMLifecycle(t *testing.T) {
	if _, err := exec.LookPath("limactl"); err != nil {
		t.Skip("limactl not installed")
	}
	name := os.Getenv("DEVVM_E2E_NAME")
	if name == "" {
		name = "e2e"
	}
	devvm := buildDevVM(t)

	// A failed assertion must not leave the VM behind for the next run.
	t.Cleanup(func() {
		exec.Command(devvm, "destroy", name, "-github-key=false").Run()
	})

	mustRun(t, devvm, "create", name, "-github-key=false")
	checkDocker(t, name)
	checkShell(t, name)
	checkMise(t, name)

	mustRun(t, "limactl", "stop", name)
	mustRun(t, "limactl", "start", "--tty=false", name)
	checkDocker(t, name)

	mustRun(t, devvm, "destroy", name, "-github-key=false")
	if vmExists(name) {
		t.Fatalf("VM %q still exists after destroy", name)
	}
}

func checkDocker(t *testing.T, name string) {
	t.Helper()
	out := guest(t, name, "docker", "info")
	if !strings.Contains(out, "Server Version:") {
		t.Errorf("docker server not running; docker info:\n%s", out)
	}
}

func checkShell(t *testing.T, name string) {
	t.Helper()
	out := guest(t, name, "sh", "-c", `getent passwd "$(id -un)" | cut -d: -f7`)
	if out != "/usr/bin/zsh" {
		t.Errorf("login shell = %q, want /usr/bin/zsh", out)
	}
}

func checkMise(t *testing.T, name string) {
	t.Helper()
	out := guest(t, name, "mise", "--version")
	if !strings.Contains(out, "mise") {
		t.Errorf("mise not installed; mise --version:\n%s", out)
	}
}

func buildDevVM(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "devvm")
	mustRun(t, "go", "build", "-o", bin, ".")
	return bin
}

// guest runs a command inside the VM. The repo directory is not mounted (the
// template has no mounts), so the working directory has to be one that exists
// in the guest.
func guest(t *testing.T, name string, args ...string) string {
	t.Helper()
	shell := append([]string{"shell", "--workdir", "/tmp", name}, args...)
	return strings.TrimSpace(mustRun(t, "limactl", shell...))
}

func mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmdline := name + " " + strings.Join(args, " ")
	t.Logf("run: %s", cmdline)
	out, err := exec.Command(name, args...).CombinedOutput()
	if len(out) > 0 {
		t.Logf("%s", out)
	}
	if err != nil {
		t.Fatalf("%s failed: %v", cmdline, err)
	}
	return string(out)
}
