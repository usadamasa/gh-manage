package reconcile

import (
	"reflect"
	"testing"
)

func TestNarrow(t *testing.T) {
	tests := []struct {
		name          string
		desired, live any
		want          any
	}{
		{
			"map は desired に無いキーを落とす",
			map[string]any{"a": "x"},
			map[string]any{"a": "y", "b": "z"},
			map[string]any{"a": "y"},
		},
		{
			"map は再帰的に絞る",
			map[string]any{"p": map[string]any{"a": "x"}},
			map[string]any{"p": map[string]any{"a": "y", "b": "z"}, "q": 1},
			map[string]any{"p": map[string]any{"a": "y"}},
		},
		{
			"desired にあって live に無いキーは出さない",
			map[string]any{"a": "x", "b": "y"},
			map[string]any{"a": "x"},
			map[string]any{"a": "x"},
		},
		{
			"長さが同じ list は要素ごとに絞る",
			[]any{map[string]any{"a": "x"}, map[string]any{"a": "x"}},
			[]any{map[string]any{"a": "x", "b": "z"}, map[string]any{"a": "y", "b": "z"}},
			[]any{map[string]any{"a": "x"}, map[string]any{"a": "y"}},
		},
		{
			"長さが違う list は live をそのまま残す",
			[]any{map[string]any{"a": "x"}},
			[]any{map[string]any{"a": "x", "b": "z"}, map[string]any{"a": "y"}},
			[]any{map[string]any{"a": "x", "b": "z"}, map[string]any{"a": "y"}},
		},
		{"scalar は live をそのまま返す", "x", "y", "y"},
		{"desired が map で live が scalar なら live をそのまま返す", map[string]any{"a": "x"}, "y", "y"},
		{"live が nil なら nil", map[string]any{"a": "x"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := narrow(tt.desired, tt.live); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("narrow() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNarrowRules(t *testing.T) {
	rule := func(typ string, params map[string]any) map[string]any {
		m := map[string]any{"type": typ}
		if params != nil {
			m["parameters"] = params
		}
		return m
	}
	tests := []struct {
		name          string
		desired, live any
		want          any
	}{
		{
			"type で突き合わせて parameters を絞る",
			[]any{rule("pull_request", map[string]any{"count": 1})},
			[]any{rule("pull_request", map[string]any{"count": 0, "reviewers": []any{}})},
			[]any{rule("pull_request", map[string]any{"count": 0})},
		},
		{
			"desired に parameters が無ければ live の parameters を残す",
			[]any{rule("deletion", nil)},
			[]any{rule("deletion", map[string]any{"x": 1})},
			[]any{rule("deletion", map[string]any{"x": 1})},
		},
		{
			"desired の type の順に並べ､desired に無い type は live の順で末尾に回す",
			[]any{rule("pull_request", map[string]any{"count": 1}), rule("deletion", nil)},
			[]any{rule("update", nil), rule("deletion", nil), rule("non_fast_forward", nil), rule("pull_request", map[string]any{"count": 0, "r": 1})},
			[]any{rule("pull_request", map[string]any{"count": 0}), rule("deletion", nil), rule("update", nil), rule("non_fast_forward", nil)},
		},
		{
			"live に parameters が無ければ足さない",
			[]any{rule("pull_request", map[string]any{"count": 1})},
			[]any{rule("pull_request", nil)},
			[]any{rule("pull_request", nil)},
		},
		{"live が list でなければそのまま返す", []any{rule("deletion", nil)}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := narrowRules(tt.desired, tt.live); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("narrowRules() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
