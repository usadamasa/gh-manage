package reconcile

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/usadamasa/gh-manage/internal/config"
)

var update = flag.Bool("update", false, "rewrite golden files")

func readSettings(t *testing.T, path string) *config.Settings {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var s config.Settings
	if err := yaml.Unmarshal(data, &s); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return &s
}

// TestPlan_Golden compares plans of testdata/plan/<case>/{desired,live}.yaml with want.txt.
// live.yaml が無いケースはリポジトリが存在しない扱い｡
func TestPlan_Golden(t *testing.T) {
	dirs, err := filepath.Glob("testdata/plan/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			desired := readSettings(t, filepath.Join(dir, "desired.yaml"))
			live := readSettings(t, filepath.Join(dir, "live.yaml"))
			p := Plan(name, desired, live)

			var buf bytes.Buffer
			if err := WriteText(&buf, []RepoPlan{p}); err != nil {
				t.Fatal(err)
			}
			wantPath := filepath.Join(dir, "want.txt")
			if *update {
				if err := os.WriteFile(wantPath, buf.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != string(want) {
				t.Errorf("plan =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestPlan_HasChanges(t *testing.T) {
	tests := []struct {
		name string
		plan RepoPlan
		want bool
	}{
		{"変更なし", Plan("r", &config.Settings{}, &config.Settings{}), false},
		{"notice だけなら変更なし", Plan("r", &config.Settings{}, &config.Settings{Variables: config.Variables{"A": {Value: "b"}}}), false},
		{"skip は変更なし", Skip("r", "archived"), false},
		{"create は変更あり", Plan("r", &config.Settings{}, nil), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.HasChanges(); got != tt.want {
				t.Errorf("HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteText_Skip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteText(&buf, []RepoPlan{Skip("old", "archived なので skip")}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "old:\n  ! archived なので skip\n"; got != want {
		t.Errorf("WriteText() = %q, want %q", got, want)
	}
}

func TestWriteJSON(t *testing.T) {
	desired := &config.Settings{Variables: config.Variables{"A": {Value: "new"}}}
	live := &config.Settings{Variables: config.Variables{"A": {Value: "old"}}}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, []RepoPlan{Plan("r", desired, live)}); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	want := `[{"repo":"r","changes":[{"op":"update","kind":"variable","name":"A","old":"old","new":"new"}]}]`
	if strings.TrimSpace(buf.String()) != want {
		t.Errorf("WriteJSON() =\n%s\nwant\n%s", buf.String(), want)
	}
}
