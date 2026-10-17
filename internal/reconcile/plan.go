// Package reconcile computes the difference between desired and live settings.
package reconcile

import (
	"errors"
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
	p.planTopics(desiredTopics(desired, live.Topics), live.Topics)
	p.planRulesets(desired, live)
	p.planVariables(desired, live)
	p.planSecrets(KindSecret, "secrets", desired.Secrets, live.Secrets, desired.Data.Secrets, desired.Prune.Secrets)
	p.planSecrets(KindDependabotSecret, "dependabot_secrets", desired.DependabotSecrets, live.DependabotSecrets, desired.Data.DependabotSecrets, desired.Prune.DependabotSecrets)
	return *p
}

// CheckData reports every data resource that live does not have. live が nil ならリポジトリが無いので全部無い｡
func CheckData(repo string, desired, live *config.Settings) error {
	if live == nil {
		live = &config.Settings{}
	}
	var errs []error
	errs = append(errs, missingData(repo, "ruleset", desired.Data.Rulesets, live.Rulesets)...)
	errs = append(errs, missingData(repo, "variable", desired.Data.Variables, live.Variables)...)
	errs = append(errs, missingData(repo, KindSecret, desired.Data.Secrets, live.Secrets)...)
	errs = append(errs, missingData(repo, KindDependabotSecret, desired.Data.DependabotSecrets, live.DependabotSecrets)...)
	return errors.Join(errs...)
}

func missingData[V any](repo, kind string, names []string, live map[string]V) []error {
	var errs []error
	for _, name := range names {
		if _, ok := live[name]; !ok {
			errs = append(errs, fmt.Errorf("%s: data %s %s が live に無い", repo, kind, name))
		}
	}
	return errs
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

// desiredTopics returns the topics a repository should have: the declared ones plus the managed topic.
// topics を宣言していなければ live の topics を土台にして marker だけ足す｡どちらも無ければ nil (比べない)｡
func desiredTopics(desired *config.Settings, live []string) []string {
	if desired.ManagedTopic == "" {
		return desired.Topics
	}
	base := desired.Topics
	if base == nil {
		base = live
	}
	out := sortedCopy(base)
	if !slices.Contains(out, desired.ManagedTopic) {
		out = append(out, desired.ManagedTopic)
		sort.Strings(out)
	}
	return out
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
			equal, old := subset(dv, lg[key]), narrow(dv, lg[key])
			if key == "rules" {
				equal, old = rulesEqual(dv, lg[key]), narrowRules(dv, lg[key])
			}
			if !equal {
				p.add(Change{Op: OpUpdate, Kind: "ruleset", Name: name, Key: key, Old: old, New: dv})
			}
		}
	}
	for _, name := range sortedKeys(live.Rulesets) {
		if _, ok := desired.Rulesets[name]; !ok && !slices.Contains(desired.Data.Rulesets, name) {
			p.undeclared("ruleset", "rulesets", name, desired.Prune.Rulesets)
		}
	}
}

func (p *RepoPlan) planVariables(desired, live *config.Settings) {
	for _, name := range sortedKeys(desired.Variables) {
		d := desired.Variables[name]
		l, ok := live.Variables[name]
		switch {
		case !ok:
			p.add(Change{Op: OpCreate, Kind: "variable", Name: name})
		case d.FromEnv != "" && d.Value == "":
			// 環境変数が無いと値が分からない｡apply はこの状態で止まるので､plan は比べずに進める
			p.Notices = append(p.Notices, fmt.Sprintf("variable %s は環境変数 %s が無いので値を比べない", name, d.FromEnv))
		case l.Value != d.Value:
			p.add(Change{Op: OpUpdate, Kind: "variable", Name: name, Old: l.Value, New: d.Value})
		}
	}
	for _, name := range sortedKeys(live.Variables) {
		if _, ok := desired.Variables[name]; !ok && !slices.Contains(desired.Data.Variables, name) {
			p.undeclared("variable", "variables", name, desired.Prune.Variables)
		}
	}
}

// planSecrets compares only presence: the API does not return secret values.
func (p *RepoPlan) planSecrets(kind, section string, desired, live config.Secrets, data []string, prune bool) {
	for _, name := range sortedKeys(desired) {
		if _, ok := live[name]; !ok {
			p.add(Change{Op: OpCreate, Kind: kind, Name: name})
		}
	}
	for _, name := range sortedKeys(live) {
		if _, ok := desired[name]; !ok && !slices.Contains(data, name) {
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
