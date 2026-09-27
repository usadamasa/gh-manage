package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRenderSettings(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"base.yaml":       "repository:\n  has_wiki: false\n",
		"repos/foo.yaml":  "repository:\n  visibility: private\n",
		"repos/bar.yaml":  "{}\n",
		"repos/notes.txt": "ignored\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const prunedDefaults = `prune:
  rulesets: false
  variables: false
  secrets: false
  dependabot_secrets: false
`

func TestRenderCmd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{
			name: "名前を指定すると 1 リポジトリ分を出す",
			args: []string{"foo"},
			want: prunedDefaults + "repository:\n  visibility: private\n  has_wiki: false\n",
		},
		{
			name: "名前を省くと全リポジトリを名前の順に出す",
			args: nil,
			want: "bar:\n" + indent(prunedDefaults+"repository:\n  has_wiki: false\n") +
				"foo:\n" + indent(prunedDefaults+"repository:\n  visibility: private\n  has_wiki: false\n"),
		},
		{
			name:    "管理していない名前はエラー",
			args:    []string{"baz"},
			wantErr: "baz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeRenderSettings(t)
			root := newRootCmd("test")
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetArgs(append([]string{"render", "--settings", dir}, tt.args...))

			err := root.Execute()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Execute() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got := out.String(); got != tt.want {
				t.Errorf("output =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func indent(s string) string {
	lines := strings.SplitAfter(s, "\n")
	var b strings.Builder
	for _, l := range lines {
		if l != "" {
			b.WriteString("  " + l)
		}
	}
	return b.String()
}
