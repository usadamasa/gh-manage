package reconcile

import (
	"cmp"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
)

// asMap converts v to its JSON form (map[string]any, []any, float64, string, bool, nil).
func asMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// subset reports whether every key written in desired has the same value in live, recursively.
// live にだけあるキーは無視する｡desired の空の list / map は live に無くても一致とみなす｡
func subset(desired, live any) bool {
	switch d := desired.(type) {
	case map[string]any:
		l, ok := live.(map[string]any)
		if !ok {
			return live == nil && len(d) == 0
		}
		return mapSubset(d, l)
	case []any:
		l, ok := live.([]any)
		if !ok {
			return live == nil && len(d) == 0
		}
		return listSubset(d, l)
	default:
		return reflect.DeepEqual(desired, live)
	}
}

func mapSubset(desired, live map[string]any) bool {
	for k, dv := range desired {
		if !subset(dv, live[k]) {
			return false
		}
	}
	return true
}

// listSubset compares lists element by element: list は丸ごと宣言するので長さも一致させる｡
func listSubset(desired, live []any) bool {
	if len(desired) != len(live) {
		return false
	}
	for i := range desired {
		if !subset(desired[i], live[i]) {
			return false
		}
	}
	return true
}

// narrow returns live with the keys that subset does not look at removed, so that a diff shows only what the comparison saw.
// 範囲は subset と同じ: map は desired に無いキーを落とし､list は長さが同じときだけ要素ごとに絞る｡
func narrow(desired, live any) any {
	switch d := desired.(type) {
	case map[string]any:
		l, ok := live.(map[string]any)
		if !ok {
			return live
		}
		out := make(map[string]any, len(d))
		for k, dv := range d {
			if lv, found := l[k]; found {
				out[k] = narrow(dv, lv)
			}
		}
		return out
	case []any:
		l, ok := live.([]any)
		if !ok || len(d) != len(l) {
			return live
		}
		out := make([]any, len(l))
		for i := range l {
			out[i] = narrow(d[i], l[i])
		}
		return out
	default:
		return live
	}
}

// narrowRules narrows the parameters of each live rule by the desired rule of the same type,
// and orders the rules as desired does so that the diff shows only parameter changes.
// desired に parameters が無い rule と desired に無い type は rulesEqual が差分にするので live のまま残し､
// desired に無い type は live の順で末尾に回す｡
func narrowRules(desired, live any) any {
	l, ok := live.([]any)
	if !ok {
		return live
	}
	d, _ := desired.([]any)
	paramsByType := map[any]any{}
	rank := map[any]int{}
	for i, r := range d {
		if m, ok := r.(map[string]any); ok {
			rank[m["type"]] = i
			if m["parameters"] != nil {
				paramsByType[m["type"]] = m["parameters"]
			}
		}
	}
	rankOf := func(r any) int {
		if m, ok := r.(map[string]any); ok {
			if i, found := rank[m["type"]]; found {
				return i
			}
		}
		return len(d)
	}
	out := make([]any, len(l))
	for i, r := range l {
		out[i] = r
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		dp, found := paramsByType[m["type"]]
		lp, has := m["parameters"]
		if !found || !has {
			continue
		}
		rule := maps.Clone(m)
		rule["parameters"] = narrow(dp, lp)
		out[i] = rule
	}
	slices.SortStableFunc(out, func(a, b any) int { return cmp.Compare(rankOf(a), rankOf(b)) })
	return out
}

// rulesEqual matches rules by type and compares their parameters with subset.
// rules は list 全体を宣言するので､live にだけある type も差分になる｡
func rulesEqual(desired, live any) bool {
	d, _ := desired.([]any)
	l, _ := live.([]any)
	if len(d) != len(l) {
		return false
	}
	byType := map[any]any{}
	for _, r := range l {
		if m, ok := r.(map[string]any); ok {
			byType[m["type"]] = m["parameters"]
		}
	}
	for _, r := range d {
		m, ok := r.(map[string]any)
		if !ok {
			return false
		}
		params, found := byType[m["type"]]
		if !found || !subset(m["parameters"], params) {
			return false
		}
	}
	return true
}
