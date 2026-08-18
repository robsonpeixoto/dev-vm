package main

import "testing"

// ipJSON is `ip -4 -json addr show lima0` output from the guest.
const ipJSON = `[{"ifindex":2,"ifname":"lima0","flags":["BROADCAST","MULTICAST","UP","LOWER_UP"],` +
	`"mtu":1500,"operstate":"UP","addr_info":[{"family":"inet","local":"192.168.64.26",` +
	`"prefixlen":24,"broadcast":"192.168.64.255","scope":"global","label":"lima0"}]}]`

func TestParseGuestIP(t *testing.T) {
	for name, tc := range map[string]struct {
		out  string
		want string
	}{
		"guest output":  {ipJSON, "192.168.64.26"},
		"no interface":  {`[]`, ""},
		"no address":    {`[{"ifname":"lima0","addr_info":[]}]`, ""},
		"not json":      {"Error: instance is stopped\n", ""},
		"empty output":  {"", ""},
		"empty address": {`[{"ifname":"lima0","addr_info":[{"family":"inet","local":""}]}]`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := parseGuestIP([]byte(tc.out)); got != tc.want {
				t.Errorf("parseGuestIP(%q) = %q, want %q", tc.out, got, tc.want)
			}
		})
	}
}

func TestInstanceIface(t *testing.T) {
	var inst limaInstance
	if got := inst.iface(); got != "lima0" {
		t.Errorf("iface() with no network = %q, want %q", got, "lima0")
	}

	inst.Network = []struct {
		Interface string `json:"interface"`
	}{{Interface: ""}, {Interface: "lima1"}}
	if got := inst.iface(); got != "lima1" {
		t.Errorf("iface() = %q, want %q", got, "lima1")
	}
}

func TestDash(t *testing.T) {
	if got := dash(""); got != "-" {
		t.Errorf("dash(\"\") = %q, want %q", got, "-")
	}
	if got := dash("192.168.64.26"); got != "192.168.64.26" {
		t.Errorf("dash(ip) = %q, want the ip", got)
	}
}
