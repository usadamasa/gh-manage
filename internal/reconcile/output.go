package reconcile

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

var opSymbols = map[Op]string{OpCreate: "+", OpUpdate: "~", OpDelete: "-"}

// WriteText prints plans for people: `+ create` / `~ update (key: old -> new)` / `- delete` / `= no change`.
// list と map の update は値を YAML にして unified diff で出す｡
func WriteText(w io.Writer, plans []RepoPlan) error {
	var b strings.Builder
	for _, p := range plans {
		if !p.HasChanges() && len(p.Notices) == 0 {
			fmt.Fprintf(&b, "%s: = no change\n", p.Repo)
			continue
		}
		fmt.Fprintf(&b, "%s:\n", p.Repo)
		for _, c := range p.Changes {
			fmt.Fprintf(&b, "  %s\n", c.text())
			for _, l := range c.diffLines() {
				fmt.Fprintln(&b, l)
			}
		}
		for _, n := range p.Notices {
			fmt.Fprintf(&b, "  ! %s\n", n)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err //nolint:wrapcheck // 出力先のエラーはそのまま返す
}

func (c Change) text() string {
	label := c.Kind
	if c.Name != "" {
		label += " " + c.Name
	}
	if c.Key != "" {
		label += "." + c.Key
	}
	s := fmt.Sprintf("%s %s %s", opSymbols[c.Op], c.Op, label)
	switch {
	case c.Op != OpUpdate:
	case isCollection(c.New):
		s += ":"
	default:
		s += fmt.Sprintf(": %s -> %s", compact(c.Old), compact(c.New))
	}
	return s
}

// diffLines renders an update of a list or map as a YAML diff below its text line.
// GitHub の diff ハイライトは行頭の +/- だけを見るので､記号を 1 桁目に置いてから字下げする｡
func (c Change) diffLines() []string {
	if c.Op != OpUpdate || !isCollection(c.New) {
		return nil
	}
	lines := lineDiff(yamlLines(c.Old), yamlLines(c.New))
	for i, l := range lines {
		lines[i] = l[:1] + "     " + l[1:]
	}
	return lines
}

func isCollection(v any) bool {
	k := reflect.ValueOf(v).Kind()
	return k == reflect.Slice || k == reflect.Map
}

func compact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// WriteJSON prints plans as a JSON array.
func WriteJSON(w io.Writer, plans []RepoPlan) error {
	if plans == nil {
		plans = []RepoPlan{}
	}
	b, err := json.Marshal(plans)
	if err != nil {
		return fmt.Errorf("encode plan: %w", err)
	}
	_, err = fmt.Fprintln(w, string(b))
	return err //nolint:wrapcheck // 出力先のエラーはそのまま返す
}
