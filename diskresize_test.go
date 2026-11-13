package main

import (
	"testing"
)

func TestWithVMDisk(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty object",
			in:   `{}`,
			want: `{
  "vms": {
    "big": {
      "disk": 200
    }
  }
}`,
		},
		{
			name: "null vms",
			in:   `{"vms": null}`,
			want: `{
  "vms": {
    "big": {
      "disk": 200
    }
  }
}`,
		},
		{
			name: "keeps other keys and their order",
			in: `{"default": {"memory": 8, "cpus": 4, "disk": 50},
			      "vms": {"other": {"nested": true, "cpus": 2},
			              "big": {"memory": 16, "disk": 100}}}`,
			want: `{
  "default": {
    "memory": 8,
    "cpus": 4,
    "disk": 50
  },
  "vms": {
    "big": {
      "disk": 200,
      "memory": 16
    },
    "other": {
      "nested": true,
      "cpus": 2
    }
  }
}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := withVMDisk([]byte(tc.in), "big", 200)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("withVMDisk(%s) =\n%s\nwant\n%s", tc.in, got, tc.want)
			}
		})
	}
}

func TestWithVMDiskRejectsBadInput(t *testing.T) {
	for _, in := range []string{`not json`, `{"vms": []}`, `{"vms": {"big": 3}}`} {
		if _, err := withVMDisk([]byte(in), "big", 200); err == nil {
			t.Errorf("withVMDisk(%s) = nil error, want error", in)
		}
	}
}
