package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
)

const destroyUsage = `Destroy a Lima dev VM and its GitHub SSH access.

Usage: devvm destroy [name] [-github-key=false]

- Deletes the Lima VM <name>.
- Deletes the GitHub key recorded in ~/.config/dev-vm/state.json plus any
  key titled <name>, via gh. -github-key=false skips that, for VMs created
  with -github-key=false: gh is never called.
- Removes the key pair at ~/.config/dev-vm/keys/<name>.
- Drops the VM entry from the state file.

Each step is skipped with a message when there is nothing to remove, so a
partial destroy can be re-run.

`

func cmdDestroy(argv []string) {
	fs := flag.NewFlagSet("destroy", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, destroyUsage)
		fs.PrintDefaults()
	}
	var githubKey bool
	fs.BoolVar(&githubKey, "github-key", true,
		"delete the GitHub key; -github-key=false leaves GitHub alone")
	name := parseArgs(fs, argv)

	checkName(name)
	entry := getVM(name)

	deleteVM(name)
	if githubKey {
		deleteGitHubKeys(name, entry)
	} else {
		fmt.Println("skipping GitHub key deletion")
	}
	deleteKeyFiles(name, entry)

	if dropVM(name) {
		fmt.Printf("removed %s from %s\n", name, stateFile)
	} else {
		fmt.Printf("no state entry for %q, skipping\n", name)
	}
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
	checkScopes()
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
