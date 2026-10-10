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
	ActCreateVariable   ActionOp = "create_variable"
	ActUpdateVariable   ActionOp = "update_variable"
	ActDeleteVariable   ActionOp = "delete_variable"
	ActPutSecret        ActionOp = "put_secret"
	ActDeleteSecret     ActionOp = "delete_secret"
)

// Kinds of secrets, as they appear in Change.Kind and Action.Kind.
const (
	KindSecret           = "secret"
	KindDependabotSecret = "dependabot_secret" // #nosec G101 -- 種類の名前で､資格情報ではない
)

// Action is one write to GitHub. 使うフィールドは Op で決まる｡
// secret の値は持たない｡cli が Secret.FromEnv の環境変数から読む｡
type Action struct {
	Op         ActionOp
	Repository config.Repository // ActCreateRepository
	Fields     map[string]any    // ActUpdateRepository
	Topics     []string          // ActSetTopics
	Name       string            // ruleset, variable and secret actions
	Ruleset    config.Ruleset    // ActUpsertRuleset
	Value      string            // ActCreateVariable / ActUpdateVariable
	Kind       string            // ActPutSecret / ActDeleteSecret: KindSecret or KindDependabotSecret
	Secret     config.Secret     // ActPutSecret
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

// Actions turns the changes of p into writes, in order.
// desired は p を作ったときの宣言で､ruleset は宣言全体を書き込む｡
// secret は値の差分が見えないので､宣言したものを差分の有無にかかわらず毎回書き直す｡
func Actions(p RepoPlan, desired *config.Settings) ([]Action, error) {
	if p.Skipped {
		return nil, nil
	}
	if len(p.Changes) == 1 && p.Changes[0].Kind == "repository" && p.Changes[0].Op == OpCreate {
		return createActions(desired)
	}

	var acts []Action
	fields := map[string]any{}
	var topics []string
	var upserts, deletes []string
	for _, c := range p.Changes {
		switch {
		case c.Kind == "repository":
			fields[c.Key] = c.New
		case c.Kind == "topics":
			// plan が managed topic を足した結果を書く
			topics, _ = c.New.([]string)
		case c.Kind == "ruleset" && c.Op == OpDelete:
			deletes = append(deletes, c.Name)
		case c.Kind == "ruleset" && !slices.Contains(upserts, c.Name):
			upserts = append(upserts, c.Name)
		}
	}
	if len(fields) > 0 {
		acts = append(acts, Action{Op: ActUpdateRepository, Fields: fields})
	}
	if topics != nil {
		acts = append(acts, Action{Op: ActSetTopics, Topics: topics})
	}
	acts = append(acts, rulesetActions(desired, upserts, deletes)...)
	acts = append(acts, variableActions(p.Changes, desired.Variables)...)
	acts = append(acts, secretActions(KindSecret, p.Changes, desired.Secrets)...)
	return append(acts, secretActions(KindDependabotSecret, p.Changes, desired.DependabotSecrets)...), nil
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
	if topics := desiredTopics(desired, nil); topics != nil {
		acts = append(acts, Action{Op: ActSetTopics, Topics: topics})
	}
	acts = append(acts, rulesetActions(desired, sortedKeys(desired.Rulesets), nil)...)
	for _, name := range sortedKeys(desired.Variables) {
		acts = append(acts, Action{Op: ActCreateVariable, Name: name, Value: desired.Variables[name].Value})
	}
	acts = append(acts, secretActions(KindSecret, nil, desired.Secrets)...)
	return append(acts, secretActions(KindDependabotSecret, nil, desired.DependabotSecrets)...), nil
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

// variableOps maps the Op of a variable Change to its write.
var variableOps = map[Op]ActionOp{OpCreate: ActCreateVariable, OpUpdate: ActUpdateVariable, OpDelete: ActDeleteVariable}

func variableActions(changes []Change, desired config.Variables) []Action {
	var acts []Action
	for _, c := range changes {
		if c.Kind == "variable" {
			acts = append(acts, Action{Op: variableOps[c.Op], Name: c.Name, Value: desired[c.Name].Value})
		}
	}
	return acts
}

// secretActions puts every declared secret of kind and then deletes the ones the plan prunes.
func secretActions(kind string, changes []Change, desired config.Secrets) []Action {
	var acts []Action
	for _, name := range sortedKeys(desired) {
		acts = append(acts, Action{Op: ActPutSecret, Kind: kind, Name: name, Secret: desired[name]})
	}
	for _, c := range changes {
		if c.Kind == kind && c.Op == OpDelete {
			acts = append(acts, Action{Op: ActDeleteSecret, Kind: kind, Name: c.Name})
		}
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
