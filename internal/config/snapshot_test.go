package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEncodeOverlay(t *testing.T) {
	live := baseSettings()
	got, err := EncodeOverlay(live)
	if err != nil {
		t.Fatalf("EncodeOverlay() error = %v", err)
	}
	want := `repository:
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
		t.Errorf("EncodeOverlay() =\n%s\nwant\n%s", got, want)
	}
}

func TestMinimize(t *testing.T) {
	tests := []struct {
		name string
		base string
		live func(s *Settings)
		want string
	}{
		{
			name: "base と同じなら {}",
			live: func(*Settings) {},
			want: "{}\n",
		},
		{
			name: "違うフィールドだけ残す",
			live: func(s *Settings) {
				s.Repository.Visibility = ptr("private")
				s.Repository.Description = ptr("")
			},
			want: "repository:\n  description: \"\"\n  visibility: private\n",
		},
		{
			name: "live に無い secret は null で消す",
			live: func(s *Settings) { s.Secrets = Secrets{"X": {FromEnv: "X"}} },
			want: "secrets:\n  X:\n    from_env: X\n  TAGPR_PRIVATE_KEY: null\n",
		},
		{
			name: "live に ruleset が無ければセクションごと null",
			live: func(s *Settings) { s.Rulesets = nil },
			want: "rulesets: null\n",
		},
		{
			name: "rules が違えば list 全体を書く",
			live: func(s *Settings) {
				rs := s.Rulesets["main"]
				rs.Rules = []Rule{{Type: "deletion"}}
				s.Rulesets["main"] = rs
			},
			want: "rulesets:\n  main:\n    rules:\n      - type: deletion\n",
		},
		{
			name: "scalar は書き方でなく値で比べる",
			base: testBase + "variables:\n  GO_VERSION: 1.10\n",
			live: func(s *Settings) { s.Variables = map[string]string{"GO_VERSION": "1.10"} },
			want: "{}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := tt.base
			if base == "" {
				base = testBase
			}
			dir := writeSettings(t, base, nil)
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			live := baseSettings()
			if tt.base != "" {
				live.Variables = map[string]string{"GO_VERSION": "1.10"}
			}
			tt.live(live)

			got, err := cfg.Minimize(live)
			if err != nil {
				t.Fatalf("Minimize() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Minimize() =\n%s\nwant\n%s", got, tt.want)
			}

			// 書き出した overlay を render すると live に戻る
			if err := os.WriteFile(filepath.Join(dir, "repos", "foo.yaml"), got, 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err = Load(dir)
			if err != nil {
				t.Fatalf("Load() after write error = %v", err)
			}
			rendered, err := cfg.Render("foo")
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			normalize(live)
			normalize(rendered)
			if !reflect.DeepEqual(rendered, live) {
				t.Errorf("Render(Minimize(live)) =\n%#v\nwant\n%#v", rendered, live)
			}
		})
	}
}

// normalize treats empty and nil collections as the same, as YAML does.
func normalize(s *Settings) {
	if len(s.Secrets) == 0 {
		s.Secrets = nil
	}
	if len(s.Rulesets) == 0 {
		s.Rulesets = nil
	}
}
