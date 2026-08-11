package main

import (
	"strings"
	"testing"
)

func TestKeyTitle(t *testing.T) {
	title := keyTitle("default")
	parts := strings.Split(title, "/")
	if len(parts) != 3 {
		t.Fatalf("keyTitle(default) = %q, want three /-separated parts", title)
	}
	if parts[0] != "dev-vm" {
		t.Errorf("prefix = %q, want dev-vm", parts[0])
	}
	if parts[1] == "" || strings.Contains(parts[1], ".") {
		t.Errorf("host = %q, want a non-empty short host name", parts[1])
	}
	if parts[2] != "default" {
		t.Errorf("name = %q, want default", parts[2])
	}
}

func TestHostNameIsShort(t *testing.T) {
	host := hostName()
	if host == "" {
		t.Fatal("hostName() = \"\", want a host name")
	}
	if strings.Contains(host, ".") {
		t.Errorf("hostName() = %q, want no domain suffix", host)
	}
}
