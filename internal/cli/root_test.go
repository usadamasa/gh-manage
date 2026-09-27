package cli

import (
	"bytes"
	"testing"
)

func TestRootCmd_Version(t *testing.T) {
	root := newRootCmd("v1.2.3 (commit: abc1234)")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "gh-manage v1.2.3 (commit: abc1234)\n"
	if got := out.String(); got != want {
		t.Errorf("--version output = %q, want %q", got, want)
	}
}
