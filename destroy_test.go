package main

import (
	"io"
	"strings"
	"testing"
)

func TestConfirmDestroy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{name: "exact name", answer: "myvm\n"},
		{name: "trailing spaces", answer: "  myvm  \n"},
		{name: "no newline", answer: "myvm"},
		{name: "other name", answer: "othervm\n", wantErr: true},
		{name: "yes", answer: "y\n", wantErr: true},
		{name: "empty", answer: "\n", wantErr: true},
		{name: "eof", answer: "", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := confirmDestroy("myvm", strings.NewReader(tc.answer), io.Discard)
			if (err != nil) != tc.wantErr {
				t.Errorf("confirmDestroy(%q) = %v, wantErr %v", tc.answer, err, tc.wantErr)
			}
		})
	}
}

// TestConfirmDestroyPrompt: the prompt has to name the VM, since typing that
// name is what the user is being asked for.
func TestConfirmDestroyPrompt(t *testing.T) {
	var out strings.Builder
	if err := confirmDestroy("myvm", strings.NewReader("myvm\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "myvm") {
		t.Errorf("prompt %q does not name the VM", out.String())
	}
}
