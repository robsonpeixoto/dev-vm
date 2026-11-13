package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

const deleteUsage = `Delete a Lima dev VM and its GitHub SSH access.

Usage: devvm delete [name] [-force]

- First, makes sure that the gh token has the admin:public_key scope. As a
  result, an authentication error stops the command before it deletes anything.
- Next, asks for confirmation. You must type the VM name. -force skips the
  prompt. If stdin is not a terminal, -force is required.
- Deletes, through gh, the GitHub key that ~/.config/dev-vm/state.json
  records. It also deletes each key with the title that the file records. That
  title is dev-vm/<host>/<name>, or only <name> for VMs from before the host
  was part of the title.
- Deletes the key pair at ~/.config/dev-vm/keys/<name>.
- Deletes the Lima VM <name>.
- Deletes the VM entry from the state file.

The steps that you can recover from come first, and the VM disk comes last. As
a result, the VM stays in place after a failure in a GitHub step. If a step has
nothing to delete, the command shows a message and skips the step. Thus you can
run a partial delete again.

`

func cmdDelete(argv []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, deleteUsage)
		fs.PrintDefaults()
	}
	var force bool
	fs.BoolVar(&force, "force", false,
		"skip the confirmation prompt. If stdin is not a terminal, this flag is required")
	name := parseArgs(fs, argv)

	checkName(name)
	checkScopes()
	if !force {
		if !isTerminal(os.Stdin) {
			die("delete needs a terminal to confirm; rerun with: devvm delete %s -force", name)
		}
		if err := confirmDelete(name, os.Stdin, os.Stdout); err != nil {
			die("%v", err)
		}
	}
	entry := getVM(name)

	deleteGitHubKeys(name, entry)
	deleteKeyFiles(name, entry)
	deleteVM(name)

	if dropVM(name) {
		fmt.Printf("removed %s from %s\n", name, stateFile)
	} else {
		fmt.Printf("no state entry for %q, skipping\n", name)
	}
}

// confirmDelete requires the VM name to be typed back before anything is
// deleted. A short read counts as a mismatch: ^D leaves the VM alone.
func confirmDelete(name string, in io.Reader, out io.Writer) error {
	fmt.Fprintf(out, "This deletes VM %q, its disk and everything in it, "+
		"its GitHub key and the local key pair.\n", name)
	fmt.Fprintf(out, "Type %q to confirm: ", name)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("cannot read confirmation: %v", err)
	}
	if strings.TrimSpace(answer) != name {
		return errors.New("name does not match, nothing deleted")
	}
	return nil
}

// isTerminal reports whether the file is a character device, which for stdin
// means an interactive terminal rather than a pipe or a redirected file.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func deleteVM(name string) {
	if !vmExists(name) {
		fmt.Printf("no VM %q, skipping\n", name)
		return
	}
	limactlRun("delete", "-f", name)
	fmt.Printf("deleted VM %s\n", name)
}

func deleteGitHubKeys(name string, entry map[string]any) {
	previous, _ := keyID(entry["github_key_id"])
	title, _ := entry["github_key_title"].(string)
	if title == "" {
		title = name
	}
	var targets []ghKey
	for _, k := range listKeys() {
		if k.Title == title || k.ID == previous {
			targets = append(targets, k)
		}
	}
	if len(targets) == 0 {
		fmt.Printf("no GitHub key for %q, skipping\n", name)
		return
	}
	for _, k := range targets {
		deleteKey(k.ID)
		fmt.Printf("deleted GitHub key %d (%s)\n", k.ID, k.Title)
	}
}

func deleteKeyFiles(name string, entry map[string]any) {
	defaultKey, defaultPub := keyPaths(name)
	paths := []string{defaultKey, defaultPub}
	for _, field := range []string{"private_key", "public_key"} {
		if p, _ := entry[field].(string); p != "" && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	removed := false
	for _, p := range paths {
		if !fileExists(p) {
			continue
		}
		if err := os.Remove(p); err != nil {
			die("cannot remove %s: %v", p, err)
		}
		fmt.Printf("removed %s\n", p)
		removed = true
	}
	if !removed {
		fmt.Printf("no key files at %s, skipping\n", defaultKey)
	}
}
