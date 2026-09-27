package reconcile

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/usadamasa/gh-manage/internal/config"
)

// ActionOp is what an Action writes.
type ActionOp string

// Operations of an Action. 呼び出し側 (cli) が github.Client のメソッドに対応付けて実行する｡
const (
	ActCreateRepository ActionOp = "create_repository"
	ActUpdateRepository ActionOp = "update_repository"
	ActSetTopics        ActionOp = "set_topics"
	ActUpsertRuleset    ActionOp = "upsert_ruleset"
	ActDeleteRuleset    ActionOp = "delete_ruleset"
)

// Action is one write to GitHub. 使うフィールドは Op で決まる｡
type Action struct {
	Op         ActionOp
	Repository config.Repository // ActCreateRepository
	Fields     map[string]any    // ActUpdateRepository
	Topics     []string          // ActSetTopics
	Name       string            // ActUpsertRuleset / ActDeleteRuleset
	Ruleset    config.Ruleset    // ActUpsertRuleset
}

// CheckPublish rejects plans that turn a private repository public.
// apply の前に呼び､--allow-publish が無ければ 1 件も書き込まずに止める｡
func CheckPublish(plans []RepoPlan) error {
	var errs []error
	for _, p := range plans {
		for _, c := range p.Changes {
			if c.Kind == "repository" && c.Key == "visibility" && c.Old == "private" && c.New == "public" {
				errs = append(errs, fmt.Errorf("%s: private を public にする変更は --allow-publish が要る", p.Repo))
			}
		}
	}
	return errors.Join(errs...)
}

// Actions turns the repository, topics and ruleset changes of p into writes, in order.
// desired は p を作ったときの宣言で､ruleset は宣言全体を書き込む｡
func Actions(p RepoPlan, desired *config.Settings) ([]Action, error) {
	if p.Skipped || !p.HasChanges() {
		return nil, nil
	}
	if len(p.Changes) == 1 && p.Changes[0].Kind == "repository" && p.Changes[0].Op == OpCreate {
		return createActions(desired)
	}

	var acts []Action
	fields := map[string]any{}
	var topics bool
	var upserts, deletes []string
	for _, c := range p.Changes {
		switch {
		case c.Kind == "repository":
			fields[c.Key] = c.New
		case c.Kind == "topics":
			topics = true
		case c.Kind == "ruleset" && c.Op == OpDelete:
			deletes = append(deletes, c.Name)
		case c.Kind == "ruleset" && !slices.Contains(upserts, c.Name):
			upserts = append(upserts, c.Name)
		}
	}
	if len(fields) > 0 {
		acts = append(acts, Action{Op: ActUpdateRepository, Fields: fields})
	}
	if topics {
		acts = append(acts, Action{Op: ActSetTopics, Topics: desired.Topics})
	}
	return append(acts, rulesetActions(desired, upserts, deletes)...), nil
}

// createActions makes the repository and then writes every declared setting.
func createActions(desired *config.Settings) ([]Action, error) {
	acts := []Action{{Op: ActCreateRepository, Repository: desired.Repository}}
	fields, err := declaredFields(desired.Repository)
	if err != nil {
		return nil, err
	}
	if len(fields) > 0 {
		acts = append(acts, Action{Op: ActUpdateRepository, Fields: fields})
	}
	if desired.Topics != nil {
		acts = append(acts, Action{Op: ActSetTopics, Topics: desired.Topics})
	}
	return append(acts, rulesetActions(desired, sortedKeys(desired.Rulesets), nil)...), nil
}

func rulesetActions(desired *config.Settings, upserts, deletes []string) []Action {
	var acts []Action
	for _, name := range upserts {
		acts = append(acts, Action{Op: ActUpsertRuleset, Name: name, Ruleset: desired.Rulesets[name]})
	}
	for _, name := range deletes {
		acts = append(acts, Action{Op: ActDeleteRuleset, Name: name})
	}
	return acts
}

// declaredFields returns the non-nil fields of r keyed by their API names.
func declaredFields(r config.Repository) (map[string]any, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("encode repository: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("encode repository: %w", err)
	}
	return m, nil
}
