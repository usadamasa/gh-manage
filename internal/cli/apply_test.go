package cli

import (
	"bytes"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/usadamasa/gh-manage/internal/github"
)

// recordingAPI answers GET from fakeAPI and records every other request.
type recordingAPI struct {
	get   fakeAPI
	calls []string
}

func (r *recordingAPI) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet {
		return r.get.RoundTrip(req)
	}
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	// 封緘した secret は毎回変わるので､平文が載っていないことだけを見て伏せる
	if bytes.Contains(body, []byte("encrypted_value")) {
		if bytes.Contains(body, []byte(secretValue)) {
			r.calls = append(r.calls, "LEAKED "+string(body))
		}
		body = []byte("<sealed>")
	}
	r.calls = append(r.calls, strings.TrimSpace(req.Method+" "+strings.TrimPrefix(req.URL.Path, "/")+" "+string(body)))
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    req,
	}, nil
}

func execApply(t *testing.T, files map[string]string, stdin string, args ...string) (string, []string, error) {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, files)
	api := &recordingAPI{get: newFakeAPI()}
	api.get.addRepo("p", `{"visibility":"private","has_wiki":false}`)
	api.get["user/repos?affiliation=owner&per_page=100"] = `[{"name":"a"},{"name":"p"},{"name":"old","archived":true}]`
	root := newRootCmdWith("test", func() (*github.Client, error) {
		return github.New(github.Options{Token: "t", Transport: api})
	})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"apply", "--settings", dir}, args...))
	err := root.Execute()
	if strings.Contains(out.String(), secretValue) {
		t.Errorf("output leaks the secret value:\n%s", out.String())
	}
	return out.String(), api.calls, err
}

// secretValue is the value of $TOKEN in the apply tests. 出力にも送信にも平文で出てはいけない｡
const secretValue = "s3cr3t-value"

// a の live は has_wiki: false (newFakeAPI)
var wikiOn = map[string]string{
	"base.yaml":    "{}\n",
	"repos/a.yaml": "repository:\n  has_wiki: true\n",
}

// withToken declares the secret TOKEN that a already has.
var withToken = map[string]string{
	"base.yaml":    "secrets:\n  TOKEN:\n    from_env: TOKEN\n",
	"repos/a.yaml": "variables:\n  GO: \"1.27\"\n",
}

func TestApplyCmd(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		env       map[string]string
		stdin     string
		args      []string
		wantCalls []string
		wantOut   string
		wantErr   string
	}{
		{
			name:      "--yes なら確認せずに適用する",
			files:     wikiOn,
			args:      []string{"--yes"},
			wantCalls: []string{`PATCH repos/o/a {"has_wiki":true}`},
			wantOut:   "applied a\n",
		},
		{
			name:      "確認に y と答えると適用する",
			files:     wikiOn,
			stdin:     "y\n",
			wantCalls: []string{`PATCH repos/o/a {"has_wiki":true}`},
			wantOut:   "apply しますか? [y/N] ",
		},
		{
			name:    "確認に y 以外だと何もしない",
			files:   wikiOn,
			stdin:   "\n",
			wantOut: "apply をやめた\n",
		},
		{
			name:    "差分も secret も無ければ確認しない",
			files:   map[string]string{"base.yaml": "{}\n", "repos/a.yaml": "{}\n"},
			wantOut: "a:\n  ! secret TOKEN は宣言されていない",
		},
		{
			name:      "variable を作り､宣言した secret は封緘して毎回書き直す",
			files:     withToken,
			env:       map[string]string{"TOKEN": secretValue},
			args:      []string{"--yes"},
			wantCalls: []string{`POST repos/o/a/actions/variables {"name":"GO","value":"1.27"}`, "PUT repos/o/a/actions/secrets/TOKEN <sealed>"},
			wantOut:   "a: secret TOKEN を書き直す",
		},
		{
			name:  "secret を書き直すだけでも確認する",
			files: map[string]string{"base.yaml": "secrets:\n  TOKEN:\n    from_env: TOKEN\n", "repos/a.yaml": "{}\n"},
			env:   map[string]string{"TOKEN": secretValue},
			stdin: "\n",
			// 平文の値は出力に出さない (execApply が確かめる)
			wantOut: "apply をやめた\n",
		},
		{
			name:    "from_env の環境変数が無ければ 1 件も書かずに止める",
			files:   withToken,
			args:    []string{"--yes"},
			wantErr: "TOKEN",
		},
		{
			name: "prune.secrets なら宣言していない secret を消す",
			files: map[string]string{
				"base.yaml":    "prune:\n  secrets: true\n",
				"repos/a.yaml": "{}\n",
			},
			args:      []string{"--yes"},
			wantCalls: []string{"DELETE repos/o/a/actions/secrets/TOKEN"},
		},
		{
			name: "private → public は --allow-publish が無ければ 1 件も書かない",
			files: map[string]string{
				"base.yaml":    "{}\n",
				"repos/a.yaml": "repository:\n  has_wiki: true\n",
				"repos/p.yaml": "repository:\n  visibility: public\n",
			},
			args:    []string{"--yes"},
			wantErr: "--allow-publish",
		},
		{
			name: "--allow-publish なら public にする",
			files: map[string]string{
				"base.yaml":    "{}\n",
				"repos/p.yaml": "repository:\n  visibility: public\n",
			},
			args:      []string{"--yes", "--allow-publish"},
			wantCalls: []string{`PATCH repos/o/p {"visibility":"public"}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TOKEN", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			out, calls, err := execApply(t, tt.files, tt.stdin, tt.args...)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Errorf("calls = %q, want %q", calls, tt.wantCalls)
			}
			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("output =\n%s\nwant it to contain %q", out, tt.wantOut)
			}
		})
	}
}
