package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/usadamasa/gh-manage/internal/github"
)

// fakeAPI answers GitHub API requests from a table (httptest は sandbox で使えない)｡
type fakeAPI map[string]string

func (f fakeAPI) RoundTrip(req *http.Request) (*http.Response, error) {
	key := strings.TrimPrefix(req.URL.Path, "/")
	if req.URL.RawQuery != "" {
		key += "?" + req.URL.RawQuery
	}
	body, ok := f[key]
	status := http.StatusOK
	if !ok {
		status, body = http.StatusNotFound, `{"message":"Not Found"}`
	}
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type":          []string{"application/json"},
			"X-Ratelimit-Remaining": []string{"4321"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: req,
	}, nil
}

// addRepo registers the endpoints FetchSettings reads for o/name.
func (f fakeAPI) addRepo(name, repoJSON string) {
	p := "repos/o/" + name
	f[p] = repoJSON
	f[p+"/rulesets?per_page=100"] = `[]`
	f[p+"/actions/variables?per_page=30"] = `{"variables":[]}`
	f[p+"/actions/secrets?per_page=100"] = `{"secrets":[{"name":"TOKEN"}]}`
	f[p+"/dependabot/secrets?per_page=100"] = `{"secrets":[]}`
	f[p+"/actions/secrets/public-key"] = testPublicKey
	f[p+"/dependabot/secrets/public-key"] = testPublicKey
}

// testPublicKey is a 32-byte Curve25519 public key, enough for sealing in tests.
const testPublicKey = `{"key_id":"kid","key":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="}`

func newFakeAPI() fakeAPI {
	f := fakeAPI{
		"user": `{"login":"o"}`,
		"user/repos?affiliation=owner&per_page=100": `[{"name":"a"},{"name":"old","archived":true},{"name":"forked","fork":true}]`,
	}
	f.addRepo("a", `{"visibility":"public","has_wiki":false}`)
	f.addRepo("old", `{"visibility":"public","has_wiki":true}`)
	f.addRepo("forked", `{"visibility":"public","has_wiki":true}`)
	return f
}

func execSnapshot(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	api := newFakeAPI()
	root := newRootCmdWith("test", func() (*github.Client, error) {
		return github.New(github.Options{Token: "t", Transport: api})
	})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"snapshot", "--settings", dir}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func listOverlays(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "repos"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func readOverlay(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "repos", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSnapshotCmd_Repo(t *testing.T) {
	dir := t.TempDir()
	out, errOut, err := execSnapshot(t, dir, "--repo", "a")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "repository:\n  visibility: public\n  has_wiki: false\nsecrets:\n  TOKEN:\n    from_env: TOKEN\n"
	if got := readOverlay(t, dir, "a"); got != want {
		t.Errorf("a.yaml =\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(out, filepath.Join(dir, "repos", "a.yaml")) {
		t.Errorf("stdout = %q, want the written path", out)
	}
	if !strings.Contains(errOut, "4321") {
		t.Errorf("stderr = %q, want the rate limit", errOut)
	}
}

func TestSnapshotCmd_All(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "archived と fork は既定で除外する", args: nil, want: []string{"a.yaml"}},
		{name: "--include-archived", args: []string{"--include-archived"}, want: []string{"a.yaml", "old.yaml"}},
		{name: "--include-forks", args: []string{"--include-forks"}, want: []string{"a.yaml", "forked.yaml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if _, _, err := execSnapshot(t, dir, tt.args...); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got := listOverlays(t, dir); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("written = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSnapshotCmd_Minimize(t *testing.T) {
	dir := t.TempDir()
	base := "repository:\n  visibility: public\n  has_wiki: true\nsecrets:\n  TOKEN:\n    from_env: TOKEN\n"
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := execSnapshot(t, dir, "--repo", "a", "--minimize"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := readOverlay(t, dir, "a"), "repository:\n  has_wiki: false\n"; got != want {
		t.Errorf("a.yaml =\n%s\nwant\n%s", got, want)
	}
}

func TestSnapshotCmd_MinimizeWithoutBase(t *testing.T) {
	if _, _, err := execSnapshot(t, t.TempDir(), "--repo", "a", "--minimize"); err == nil {
		t.Error("Execute() error = nil, want error for missing base.yaml")
	}
}
