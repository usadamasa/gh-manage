package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testBase = `prune:
  rulesets: false
  secrets: true
repository:
  visibility: public
  has_wiki: false
  delete_branch_on_merge: true
rulesets:
  main:
    target: branch
    enforcement: active
    conditions:
      ref_name:
        include: ["~DEFAULT_BRANCH"]
        exclude: []
    rules:
      - type: deletion
      - type: non_fast_forward
secrets:
  TAGPR_PRIVATE_KEY:
    from_env: TAGPR_PRIVATE_KEY
`

// writeSettings creates settings/ under a temp dir. overlays maps a repo name to its YAML.
func writeSettings(t *testing.T, base string, overlays map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "repos"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range overlays {
		if err := os.WriteFile(filepath.Join(dir, "repos", name+".yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func ptr[T any](v T) *T { return &v }

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		overlays map[string]string
		wantErr  []string
	}{
		{
			name:    "base の schema に無いキー",
			base:    "repository:\n  has_wikii: false\n",
			wantErr: []string{"base.yaml", "has_wikii"},
		},
		{
			name:     "overlay の schema に無いキー",
			base:     testBase,
			overlays: map[string]string{"foo": "rulesetz: {}\n"},
			wantErr:  []string{"foo.yaml", "rulesetz"},
		},
		{
			name:     "secret に平文を書く",
			base:     testBase,
			overlays: map[string]string{"foo": "secrets:\n  TOKEN: plain-text\n"},
			wantErr:  []string{"foo.yaml", "TOKEN", "from_env"},
		},
		{
			name:     "secret に value キーを書く",
			base:     testBase,
			overlays: map[string]string{"foo": "secrets:\n  TOKEN:\n    value: plain-text\n"},
			wantErr:  []string{"foo.yaml", "value"},
		},
		{
			name:     "dependabot secret に平文を書く",
			base:     testBase,
			overlays: map[string]string{"foo": "dependabot_secrets:\n  TOKEN: plain-text\n"},
			wantErr:  []string{"foo.yaml", "TOKEN", "from_env"},
		},
		{
			name:     "variable に from_env 以外のキーを書く",
			base:     testBase,
			overlays: map[string]string{"foo": "variables:\n  FOO:\n    value: x\n"},
			wantErr:  []string{"foo.yaml", "FOO", "value"},
		},
		{
			name:     "variable の from_env が空",
			base:     testBase,
			overlays: map[string]string{"foo": "variables:\n  FOO:\n    from_env: \"\"\n"},
			wantErr:  []string{"foo.yaml", "FOO", "from_env"},
		},
		{
			name:     "ruleset の中の schema に無いキー",
			base:     testBase,
			overlays: map[string]string{"foo": "rulesets:\n  main:\n    enforcment: active\n"},
			wantErr:  []string{"foo.yaml", "enforcment"},
		},
		{
			name:     "repository.name は書けない (rename を防ぐ)",
			base:     testBase,
			overlays: map[string]string{"foo": "repository:\n  name: bar\n"},
			wantErr:  []string{"foo.yaml", "name"},
		},
		{
			name:    "YAML の構文エラー",
			base:    "repository: [\n",
			wantErr: []string{"base.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeSettings(t, tt.base, tt.overlays)
			_, err := Load(dir)
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			for _, w := range tt.wantErr {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("Load() error = %q, want it to contain %q", err, w)
				}
			}
		})
	}
}

func TestLoad_MissingBase(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("Load() error = nil, want error for missing base.yaml")
	}
}

func TestLoad_Names(t *testing.T) {
	dir := writeSettings(t, testBase, map[string]string{"zeta": "{}\n", "alpha": ""})
	if err := os.WriteFile(filepath.Join(dir, "repos", "README.md"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Names(), []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestLoad_NoReposDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte(testBase), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Names()) != 0 {
		t.Errorf("Names() = %v, want empty", cfg.Names())
	}
}

func baseSettings() *Settings {
	return &Settings{
		Prune: Prune{Rulesets: false, Secrets: true},
		Repository: Repository{
			Visibility:          ptr("public"),
			HasWiki:             ptr(false),
			DeleteBranchOnMerge: ptr(true),
		},
		Rulesets: map[string]Ruleset{
			"main": {
				Target:      "branch",
				Enforcement: "active",
				Conditions: &Conditions{RefName: &RefName{
					Include: []string{"~DEFAULT_BRANCH"},
					Exclude: []string{},
				}},
				Rules: []Rule{{Type: "deletion"}, {Type: "non_fast_forward"}},
			},
		},
		Secrets: map[string]Secret{"TAGPR_PRIVATE_KEY": {FromEnv: "TAGPR_PRIVATE_KEY"}},
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		overlay string
		want    func(s *Settings)
	}{
		{
			name:    "overlay が {} なら base がそのまま出る",
			overlay: "{}\n",
			want:    func(*Settings) {},
		},
		{
			name:    "overlay が空ファイルでも base がそのまま出る",
			overlay: "",
			want:    func(*Settings) {},
		},
		{
			name:    "repository を上書きする",
			overlay: "repository:\n  visibility: private\n  description: hello\n",
			want: func(s *Settings) {
				s.Repository.Visibility = ptr("private")
				s.Repository.Description = ptr("hello")
			},
		},
		{
			name:    "null で secret を削除する",
			overlay: "secrets:\n  TAGPR_PRIVATE_KEY: null\n",
			want:    func(s *Settings) { s.Secrets = map[string]Secret{} },
		},
		{
			name:    "rules は丸ごと置き換える",
			overlay: "rulesets:\n  main:\n    rules:\n      - type: pull_request\n        parameters:\n          required_approving_review_count: 0\n",
			want: func(s *Settings) {
				rs := s.Rulesets["main"]
				rs.Rules = []Rule{{Type: "pull_request", Parameters: map[string]any{"required_approving_review_count": 0}}}
				s.Rulesets["main"] = rs
			},
		},
		{
			name:    "prune を overlay で上書きする",
			overlay: "prune:\n  rulesets: true\n  secrets: false\n",
			want: func(s *Settings) {
				s.Prune.Rulesets = true
				s.Prune.Secrets = false
			},
		},
		{
			name:    "variable の値は書いたとおりの文字列になる",
			overlay: "variables:\n  GO_VERSION: 1.10\n",
			want:    func(s *Settings) { s.Variables = Variables{"GO_VERSION": {Value: "1.10"}} },
		},
		{
			name:    "variable は from_env でも書ける",
			overlay: "variables:\n  CLIENT_ID:\n    from_env: TAGPR_CLIENT_ID\n",
			want:    func(s *Settings) { s.Variables = Variables{"CLIENT_ID": {FromEnv: "TAGPR_CLIENT_ID"}} },
		},
		{
			name:    "variables と dependabot_secrets と topics を足す",
			overlay: "topics: [go, cli]\nvariables:\n  FOO: bar\ndependabot_secrets:\n  NPM_TOKEN:\n    from_env: NPM_TOKEN\n",
			want: func(s *Settings) {
				s.Topics = []string{"go", "cli"}
				s.Variables = Variables{"FOO": {Value: "bar"}}
				s.DependabotSecrets = map[string]Secret{"NPM_TOKEN": {FromEnv: "NPM_TOKEN"}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeSettings(t, testBase, map[string]string{"foo": tt.overlay})
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			got, err := cfg.Render("foo")
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			want := baseSettings()
			tt.want(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Render() =\n%#v\nwant\n%#v", got, want)
			}
		})
	}
}

func TestRender_UnknownRepo(t *testing.T) {
	dir := writeSettings(t, testBase, map[string]string{"foo": "{}\n"})
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := cfg.Render("bar"); err == nil || !strings.Contains(err.Error(), "bar") {
		t.Errorf("Render() error = %v, want error naming bar", err)
	}
}

func TestRender_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		overlay string
		wantErr string
	}{
		{"visibility の値", "repository:\n  visibility: secret\n", "visibility"},
		{"from_env が空", "secrets:\n  TOKEN:\n    from_env: \"\"\n", "TOKEN"},
		{"from_env が環境変数名でない", "secrets:\n  TOKEN:\n    from_env: \"a b\"\n", "TOKEN"},
		{"secret 名が不正", "secrets:\n  GITHUB_TOKEN:\n    from_env: X\n", "GITHUB_TOKEN"},
		{"variable 名が不正", "variables:\n  1FOO: x\n", "1FOO"},
		{"variable の from_env が環境変数名でない", "variables:\n  FOO:\n    from_env: \"a b\"\n", "FOO"},
		{"ruleset の enforcement", "rulesets:\n  main:\n    enforcement: on\n", "enforcement"},
		{"ruleset の target", "rulesets:\n  main:\n    target: commit\n", "target"},
		{"rule の type が空", "rulesets:\n  main:\n    rules:\n      - parameters: {}\n", "type"},
		{"ruleset の target が無い", "rulesets:\n  other:\n    enforcement: active\n", "other"},
		{"managed_topic が topic の命名規則から外れる", "managed_topic: \"Managed Topic\"\n", "managed_topic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeSettings(t, testBase, map[string]string{"foo": tt.overlay})
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			_, err = cfg.Render("foo")
			if err == nil {
				t.Fatal("Render() error = nil, want error")
			}
			for _, w := range []string{"foo", tt.wantErr} {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("Render() error = %q, want it to contain %q", err, w)
				}
			}
		})
	}
}

func TestEncode(t *testing.T) {
	got, err := Encode(baseSettings())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	want := `prune:
  rulesets: false
  variables: false
  secrets: true
  dependabot_secrets: false
repository:
  visibility: public
  has_wiki: false
  delete_branch_on_merge: true
rulesets:
  main:
    target: branch
    enforcement: active
    conditions:
      ref_name:
        include:
          - ~DEFAULT_BRANCH
        exclude: []
    rules:
      - type: deletion
      - type: non_fast_forward
secrets:
  TAGPR_PRIVATE_KEY:
    from_env: TAGPR_PRIVATE_KEY
`
	if string(got) != want {
		t.Errorf("Encode() =\n%s\nwant\n%s", got, want)
	}
}

func TestVariables_ResolveEnv(t *testing.T) {
	env := map[string]string{"CLIENT_ID": "Iv23", "EMPTY": ""}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	tests := []struct {
		name    string
		vars    Variables
		want    Variables
		wantErr []string
	}{
		{
			name: "from_env は環境変数の値を入れ､平文はそのまま",
			vars: Variables{"ID": {FromEnv: "CLIENT_ID"}, "GO": {Value: "1.27"}},
			want: Variables{"ID": {Value: "Iv23", FromEnv: "CLIENT_ID"}, "GO": {Value: "1.27"}},
		},
		{
			name:    "無いものと空のものを全部挙げる",
			vars:    Variables{"A": {FromEnv: "MISSING"}, "B": {FromEnv: "EMPTY"}},
			wantErr: []string{"A", "MISSING", "B", "EMPTY"},
		},
		{name: "nil でもよい"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.vars.ResolveEnv(lookup)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("ResolveEnv() error = nil, want error")
				}
				for _, w := range tt.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("ResolveEnv() error = %q, want it to contain %q", err, w)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveEnv() error = %v", err)
			}
			if !reflect.DeepEqual(tt.vars, tt.want) {
				t.Errorf("ResolveEnv() = %#v, want %#v", tt.vars, tt.want)
			}
		})
	}
}
