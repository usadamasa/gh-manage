package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/usadamasa/gh-manage/internal/github"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func execPlan(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	api := newFakeAPI()
	root := newRootCmdWith("test", func() (*github.Client, error) {
		return github.New(github.Options{Token: "t", Transport: api})
	})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"plan", "--settings", dir}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestPlanCmd(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		args     []string
		want     string
		wantCode int
	}{
		{
			name: "差分なしは exit 0",
			files: map[string]string{
				"base.yaml":    "repository:\n  has_wiki: false\nsecrets:\n  TOKEN:\n    from_env: TOKEN\n",
				"repos/a.yaml": "{}\n",
			},
			want:     "a: = no change\n",
			wantCode: 0,
		},
		{
			name: "差分ありは exit 2､archived は skip､無いリポジトリは create",
			files: map[string]string{
				"base.yaml":      "secrets:\n  TOKEN:\n    from_env: TOKEN\n",
				"repos/a.yaml":   "repository:\n  visibility: private\n",
				"repos/old.yaml": "{}\n",
				"repos/new.yaml": "{}\n",
			},
			want: "a:\n  ~ update repository.visibility: \"public\" -> \"private\"\n" +
				"new:\n  + create repository\n" +
				"old:\n  ! archived なので skip\n",
			wantCode: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			out, err := execPlan(t, dir, tt.args...)
			if got := ExitCode(err); got != tt.wantCode {
				t.Fatalf("ExitCode(%v) = %d, want %d", err, got, tt.wantCode)
			}
			if out != tt.want {
				t.Errorf("output =\n%s\nwant\n%s", out, tt.want)
			}
		})
	}
}

func TestPlanCmd_JSON(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"base.yaml": "{}\n", "repos/a.yaml": "repository:\n  has_wiki: true\n"})
	out, err := execPlan(t, dir, "--format", "json")
	if ExitCode(err) != 2 {
		t.Fatalf("err = %v, want drift", err)
	}
	var plans []map[string]any
	if err := json.Unmarshal([]byte(out), &plans); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(plans) != 1 || plans[0]["repo"] != "a" {
		t.Errorf("plans = %v", plans)
	}
}

func TestPlanCmd_BadFormat(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"base.yaml": "{}\n"})
	if _, err := execPlan(t, dir, "--format", "xml"); ExitCode(err) != 1 {
		t.Errorf("err = %v, want exit 1", err)
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d", got)
	}
	if got := ExitCode(errors.New("x")); got != 1 {
		t.Errorf("ExitCode(error) = %d", got)
	}
}
