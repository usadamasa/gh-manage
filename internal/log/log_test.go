package log

import (
	"bytes"
	"testing"
)

func TestLogger_SeparatesOutAndErr(t *testing.T) {
	var out, err bytes.Buffer
	l := New(&out, &err)

	l.Printf("a=%d", 1)
	l.Println("b")
	l.Errorf("c=%d", 2)
	l.Errorln("d")

	if got, want := out.String(), "a=1b\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if got, want := err.String(), "c=2d\n"; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}
