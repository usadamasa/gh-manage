package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/usadamasa/gh-manage/internal/config"
)

// Writer is what Apply needs from the GitHub API. internal/github の Client が満たす｡
type Writer interface {
	CreateRepository(ctx context.Context, name string, r config.Repository) error
	UpdateRepository(ctx context.Context, owner, name string, fields map[string]any) error
	SetTopics(ctx context.Context, owner, name string, topics []string) error
	UpsertRuleset(ctx context.Context, owner, repo, name string, rs config.Ruleset) error
	DeleteRuleset(ctx context.Context, owner, repo, name string) error
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

// Apply carries out the repository, topics and ruleset changes of p.
// desired は p を作ったときの宣言で､ruleset は宣言全体を書き込む｡
func Apply(ctx context.Context, w Writer, owner string, p RepoPlan, desired *config.Settings) error {
	if p.Skipped || !p.HasChanges() {
		return nil
	}
	if len(p.Changes) == 1 && p.Changes[0].Kind == "repository" && p.Changes[0].Op == OpCreate {
		return create(ctx, w, owner, p.Repo, desired)
	}

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
		if err := w.UpdateRepository(ctx, owner, p.Repo, fields); err != nil {
			return err
		}
	}
	if topics {
		if err := w.SetTopics(ctx, owner, p.Repo, desired.Topics); err != nil {
			return err
		}
	}
	return applyRulesets(ctx, w, owner, p.Repo, desired, upserts, deletes)
}

// create makes the repository and then writes every declared setting.
func create(ctx context.Context, w Writer, owner, repo string, desired *config.Settings) error {
	if err := w.CreateRepository(ctx, repo, desired.Repository); err != nil {
		return err
	}
	fields, err := declaredFields(desired.Repository)
	if err != nil {
		return err
	}
	if len(fields) > 0 {
		if err := w.UpdateRepository(ctx, owner, repo, fields); err != nil {
			return err
		}
	}
	if desired.Topics != nil {
		if err := w.SetTopics(ctx, owner, repo, desired.Topics); err != nil {
			return err
		}
	}
	return applyRulesets(ctx, w, owner, repo, desired, sortedKeys(desired.Rulesets), nil)
}

func applyRulesets(ctx context.Context, w Writer, owner, repo string, desired *config.Settings, upserts, deletes []string) error {
	for _, name := range upserts {
		if err := w.UpsertRuleset(ctx, owner, repo, name, desired.Rulesets[name]); err != nil {
			return err
		}
	}
	for _, name := range deletes {
		if err := w.DeleteRuleset(ctx, owner, repo, name); err != nil {
			return err
		}
	}
	return nil
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
