package reconcile

import (
	"encoding/json"
	"reflect"
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
