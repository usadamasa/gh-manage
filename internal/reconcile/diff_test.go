package reconcile

import (
	"slices"
	"strconv"
	"testing"
)

func numbered(from, to int) []string {
	var s []string
	for i := from; i <= to; i++ {
		s = append(s, strconv.Itoa(i))
	}
	return s
}

func TestLineDiff(t *testing.T) {
	tests := []struct {
		name     string
		old, new []string
		want     []string
	}{
		{"同じなら差分なし", numbered(1, 3), numbered(1, 3), nil},
		{"old が空なら全部 +", nil, []string{"a", "b"}, []string{"+a", "+b"}},
		{
			"前後 3 行の文脈を残す",
			numbered(1, 9),
			slices.Concat(numbered(1, 4), []string{"X"}, numbered(6, 9)),
			[]string{" 2", " 3", " 4", "-5", "+X", " 6", " 7", " 8"},
		},
		{
			"離れた変更は ... で区切る",
			numbered(1, 12),
			slices.Concat([]string{"A"}, numbered(2, 11), []string{"B"}),
			[]string{"-1", "+A", " 2", " 3", " 4", " ...", " 9", " 10", " 11", "-12", "+B"},
		},
		{
			"近い変更は 1 つにまとめる",
			numbered(1, 8),
			slices.Concat([]string{"A"}, numbered(2, 7), []string{"B"}),
			[]string{"-1", "+A", " 2", " 3", " 4", " 5", " 6", " 7", "-8", "+B"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineDiff(tt.old, tt.new); !slices.Equal(got, tt.want) {
				t.Errorf("lineDiff() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestYAMLLines(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want []string
	}{
		{"nil は空", nil, nil},
		{"list", []string{"cli", "go"}, []string{"- cli", "- go"}},
		{
			"map のキーは並べ替えて 2 桁で字下げする",
			map[string]any{"type": "pull_request", "parameters": map[string]any{"n": 1}},
			[]string{"parameters:", "  n: 1", "type: pull_request"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := yamlLines(tt.v); !slices.Equal(got, tt.want) {
				t.Errorf("yamlLines() = %q, want %q", got, tt.want)
			}
		})
	}
}
