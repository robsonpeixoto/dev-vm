package main

import "embed"

//go:embed lima/dev-vm.yaml lima/files lima/scripts
var assets embed.FS
