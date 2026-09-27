// Package reconcile computes the difference between desired and live settings.
package reconcile

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/usadamasa/gh-manage/internal/config"
)

// Op is what a Change does.
type Op string

// Ops of a Change.
const (
	OpCreate Op = "create"
	OpUpdate Op = "update"
	OpDelete Op = "delete"
)

// Change is one difference between desired and live.
type Change struct {
	Op   Op     `json:"op"`
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
	Old  any    `json:"old,omitempty"`
	New  any    `json:"new,omitempty"`
}

// RepoPlan is the plan of one repository.
type RepoPlan struct {
	Repo    string   `json:"repo"`
	Skipped bool     `json:"skipped,omitempty"`
	Changes []Change `json:"changes,omitempty"`
	Notices []string `json:"notices,omitempty"`
}

// HasChanges reports whether applying the plan would change anything.
func (p RepoPlan) HasChanges() bool {
	return len(p.Changes) > 0
}

// Skip is the plan of a repository that is not reconciled, such as an archived one.
func Skip(repo, reason string) RepoPlan {
	return RepoPlan{Repo: repo, Skipped: true, Notices: []string{reason}}
}

// Plan compares desired with live. live が nil ならリポジトリが存在しない｡
//
// 比較の原則は 1 つ: desired に書いたキーが live と一致すれば同じ｡live にだけあるキーは無視する｡
func Plan(repo string, desired, live *config.Settings) RepoPlan {
	p := &RepoPlan{Repo: repo}
	if live == nil {
		p.add(Change{Op: OpCreate, Kind: "repository"})
		return *p
	}
	p.planRepository(desired.Repository, live.Repository)
	p.planTopics(desired.Topics, live.Topics)
	p.planRulesets(desired, live)
	p.planVariables(desired, live)
	p.planSecrets("secret", "secrets", desired.Secrets, live.Secrets, desired.Prune.Secrets)
	p.planSecrets("dependabot_secret", "dependabot_secrets", desired.DependabotSecrets, live.DependabotSecrets, desired.Prune.DependabotSecrets)
	return *p
}

func (p *RepoPlan) add(c Change) {
	p.Changes = append(p.Changes, c)
}

// undeclared deletes a live-only resource when prune is on, and only notices it otherwise.
func (p *RepoPlan) undeclared(kind, section, name string, prune bool) {
	if prune {
		p.add(Change{Op: OpDelete, Kind: kind, Name: name})
		return
	}
	p.Notices = append(p.Notices, fmt.Sprintf("%s %s は宣言されていない (prune.%s: false なので残す)", kind, name, section))
}

// planRepository compares only the fields declared (non-nil) in desired.
func (p *RepoPlan) planRepository(desired, live config.Repository) {
	dv, lv := reflect.ValueOf(desired), reflect.ValueOf(live)
	for i := range dv.NumField() {
		d := dv.Field(i)
		if d.IsNil() {
			continue
		}
		var old any
		if l := lv.Field(i); !l.IsNil() {
			old = l.Elem().Interface()
		}
		if newValue := d.Elem().Interface(); old != newValue {
			key, _, _ := strings.Cut(dv.Type().Field(i).Tag.Get("json"), ",")
			p.add(Change{Op: OpUpdate, Kind: "repository", Key: key, Old: old, New: newValue})
		}
	}
}

// planTopics compares topics as a set.
func (p *RepoPlan) planTopics(desired, live []string) {
	if desired == nil {
		return
	}
	d, l := sortedCopy(desired), sortedCopy(live)
	if !slices.Equal(d, l) {
		p.add(Change{Op: OpUpdate, Kind: "topics", Old: l, New: d})
	}
}

// rulesetKeys is the order in which ruleset fields are compared and printed.
var rulesetKeys = []string{"target", "enforcement", "bypass_actors", "conditions", "rules"}

func (p *RepoPlan) planRulesets(desired, live *config.Settings) {
	for _, name := range sortedKeys(desired.Rulesets) {
		l, ok := live.Rulesets[name]
		if !ok {
			p.add(Change{Op: OpCreate, Kind: "ruleset", Name: name})
			continue
		}
		dg, lg := asMap(desired.Rulesets[name]), asMap(l)
		for _, key := range rulesetKeys {
			dv, declared := dg[key]
			if !declared {
				continue
			}
			equal := subset(dv, lg[key])
			if key == "rules" {
				equal = rulesEqual(dv, lg[key])
			}
			if !equal {
				p.add(Change{Op: OpUpdate, Kind: "ruleset", Name: name, Key: key, Old: lg[key], New: dv})
			}
		}
	}
	for _, name := range sortedKeys(live.Rulesets) {
		if _, ok := desired.Rulesets[name]; !ok {
			p.undeclared("ruleset", "rulesets", name, desired.Prune.Rulesets)
		}
	}
}

func (p *RepoPlan) planVariables(desired, live *config.Settings) {
	for _, name := range sortedKeys(desired.Variables) {
		l, ok := live.Variables[name]
		switch {
		case !ok:
			p.add(Change{Op: OpCreate, Kind: "variable", Name: name})
		case l != desired.Variables[name]:
			p.add(Change{Op: OpUpdate, Kind: "variable", Name: name, Old: l, New: desired.Variables[name]})
		}
	}
	for _, name := range sortedKeys(live.Variables) {
		if _, ok := desired.Variables[name]; !ok {
			p.undeclared("variable", "variables", name, desired.Prune.Variables)
		}
	}
}

// planSecrets compares only presence: the API does not return secret values.
func (p *RepoPlan) planSecrets(kind, section string, desired, live config.Secrets, prune bool) {
	for _, name := range sortedKeys(desired) {
		if _, ok := live[name]; !ok {
			p.add(Change{Op: OpCreate, Kind: kind, Name: name})
		}
	}
	for _, name := range sortedKeys(live) {
		if _, ok := desired[name]; !ok {
			p.undeclared(kind, section, name, prune)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedCopy(s []string) []string {
	out := slices.Clone(s)
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	return out
}
