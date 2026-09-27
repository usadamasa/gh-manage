package config

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func parseNode(t *testing.T, s string) *yaml.Node {
	t.Helper()
	n, err := parseMapping([]byte(s))
	if err != nil {
		t.Fatalf("parseMapping(%q) error = %v", s, err)
	}
	return n
}

func encodeNode(t *testing.T, n *yaml.Node) string {
	t.Helper()
	out, err := Encode(n)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	return string(out)
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		overlay string
		want    string
	}{
		{
			name:    "overlay が {} なら base がそのまま出る",
			base:    "repository:\n  has_wiki: false\n",
			overlay: "{}",
			want:    "repository:\n  has_wiki: false\n",
		},
		{
			name:    "overlay が空でも base がそのまま出る",
			base:    "repository:\n  has_wiki: false\n",
			overlay: "",
			want:    "repository:\n  has_wiki: false\n",
		},
		{
			name:    "map は再帰的に merge する",
			base:    "repository:\n  has_wiki: false\n  visibility: public\n",
			overlay: "repository:\n  visibility: private\n",
			want:    "repository:\n  has_wiki: false\n  visibility: private\n",
		},
		{
			name:    "null は base のキーを削除する",
			base:    "secrets:\n  A:\n    from_env: A\n  B:\n    from_env: B\n",
			overlay: "secrets:\n  A: null\n",
			want:    "secrets:\n  B:\n    from_env: B\n",
		},
		{
			name:    "~ も null として扱う",
			base:    "secrets:\n  A:\n    from_env: A\n",
			overlay: "secrets:\n  A: ~\n",
			want:    "secrets: {}\n",
		},
		{
			name:    "トップレベルの null はセクションごと削除する",
			base:    "rulesets:\n  main: {}\nprune:\n  rulesets: true\n",
			overlay: "rulesets: null\n",
			want:    "prune:\n  rulesets: true\n",
		},
		{
			name:    "base に無いキーへの null は何もしない",
			base:    "repository:\n  has_wiki: false\n",
			overlay: "repository:\n  homepage: null\n",
			want:    "repository:\n  has_wiki: false\n",
		},
		{
			name:    "list は丸ごと置き換える",
			base:    "rules:\n  - type: deletion\n  - type: non_fast_forward\n",
			overlay: "rules:\n  - type: pull_request\n",
			want:    "rules:\n  - type: pull_request\n",
		},
		{
			name:    "list の中の null は削除でなく値として残す",
			base:    "rules:\n  - type: deletion\n",
			overlay: "rules:\n  - type: code_coverage\n    parameters:\n      minimum_coverage: null\n",
			want:    "rules:\n  - type: code_coverage\n    parameters:\n      minimum_coverage: null\n",
		},
		{
			name:    "prune は overlay の値で上書きする",
			base:    "prune:\n  rulesets: false\n  secrets: true\n",
			overlay: "prune:\n  rulesets: true\n",
			want:    "prune:\n  rulesets: true\n  secrets: true\n",
		},
		{
			name:    "scalar を map で置き換える",
			base:    "a: x\n",
			overlay: "a:\n  b: 1\n",
			want:    "a:\n  b: 1\n",
		},
		{
			name:    "overlay にだけあるキーは後ろに足し､その中の null は落とす",
			base:    "z: 1\n",
			overlay: "a:\n  b: null\n  c: 1\n",
			want:    "z: 1\na:\n  c: 1\n",
		},
		{
			name:    "base 側の null も落とす",
			base:    "a: null\nb: 1\n",
			overlay: "{}",
			want:    "b: 1\n",
		},
		{
			name:    "scalar の書き方を保つ",
			base:    "variables:\n  GO_VERSION: 1.10\n  MODE: \"0755\"\n",
			overlay: "{}",
			want:    "variables:\n  GO_VERSION: 1.10\n  MODE: \"0755\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeNode(t, merge(parseNode(t, tt.base), parseNode(t, tt.overlay)))
			if got != tt.want {
				t.Errorf("merge() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestMerge_DoesNotMutateInputs(t *testing.T) {
	const baseSrc = "repository:\n  has_wiki: false\nlist:\n  - a\n"
	const overlaySrc = "repository:\n  has_wiki: true\n"
	base, overlay := parseNode(t, baseSrc), parseNode(t, overlaySrc)

	got := merge(base, overlay)
	got.Content[1].Content[1].Value = "changed"
	got.Content[3].Content[0].Value = "changed"

	if s := encodeNode(t, base); s != baseSrc {
		t.Errorf("base was mutated:\n%s", s)
	}
	if s := encodeNode(t, overlay); s != overlaySrc {
		t.Errorf("overlay was mutated:\n%s", s)
	}
}

func TestParseMapping_NotAMap(t *testing.T) {
	if _, err := parseMapping([]byte("- a\n")); err == nil {
		t.Error("parseMapping() error = nil, want error for a sequence")
	}
}
