package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/usadamasa/gh-manage/internal/config"
)

// send issues method path with v encoded as the JSON body (nil means no body).
func (c *Client) send(ctx context.Context, method, path string, v any) error {
	var body io.Reader
	if v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("%s %s: encode body: %w", method, path, err)
		}
		body = bytes.NewReader(b)
	}
	if err := c.rest.DoWithContext(ctx, method, path, body, nil); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	return nil
}

// UpdateRepository PATCHes only the given fields of owner/name.
func (c *Client) UpdateRepository(ctx context.Context, owner, name string, fields map[string]any) error {
	return c.send(ctx, http.MethodPatch, "repos/"+owner+"/"+name, fields)
}

// CreateRepository creates name under the authenticated user with an initial commit.
// 作成時は visibility と description だけを渡し､残りの設定は続く PATCH で入れる｡
func (c *Client) CreateRepository(ctx context.Context, name string, r config.Repository) error {
	body := map[string]any{"name": name, "auto_init": true}
	if r.Visibility != nil {
		body["private"] = *r.Visibility == "private"
	}
	if r.Description != nil {
		body["description"] = *r.Description
	}
	return c.send(ctx, http.MethodPost, "user/repos", body)
}

// SetTopics replaces the topics of owner/name.
func (c *Client) SetTopics(ctx context.Context, owner, name string, topics []string) error {
	if topics == nil {
		topics = []string{}
	}
	return c.send(ctx, http.MethodPut, "repos/"+owner+"/"+name+"/topics", map[string]any{"names": topics})
}

// UpsertRuleset updates the repository ruleset called rulesetName, or creates it.
func (c *Client) UpsertRuleset(ctx context.Context, owner, repo, rulesetName string, rs config.Ruleset) error {
	prefix := "repos/" + owner + "/" + repo + "/rulesets"
	ids, err := c.rulesetIDs(ctx, prefix)
	if err != nil {
		return err
	}
	body := struct {
		config.Ruleset
		Name string `json:"name"`
	}{rs, rulesetName}
	if id, ok := ids[rulesetName]; ok {
		return c.send(ctx, http.MethodPut, fmt.Sprintf("%s/%d", prefix, id), body)
	}
	return c.send(ctx, http.MethodPost, prefix, body)
}

// DeleteRuleset deletes the repository ruleset called rulesetName.
func (c *Client) DeleteRuleset(ctx context.Context, owner, repo, rulesetName string) error {
	prefix := "repos/" + owner + "/" + repo + "/rulesets"
	ids, err := c.rulesetIDs(ctx, prefix)
	if err != nil {
		return err
	}
	id, ok := ids[rulesetName]
	if !ok {
		return fmt.Errorf("%s/%s: ruleset %q が無い", owner, repo, rulesetName)
	}
	return c.send(ctx, http.MethodDelete, fmt.Sprintf("%s/%d", prefix, id), nil)
}

// rulesetIDs maps the names of the repository's own rulesets to their ids.
func (c *Client) rulesetIDs(ctx context.Context, prefix string) (map[string]int64, error) {
	list, err := getList[rulesetSummary](ctx, c, prefix+"?per_page=100")
	if err != nil {
		return nil, err
	}
	ids := map[string]int64{}
	for _, rs := range list {
		if rs.SourceType == "Repository" {
			ids[rs.Name] = rs.ID
		}
	}
	return ids, nil
}

// CreateVariable creates the Actions variable name.
func (c *Client) CreateVariable(ctx context.Context, owner, repo, name, value string) error {
	return c.send(ctx, http.MethodPost, "repos/"+owner+"/"+repo+"/actions/variables", map[string]string{"name": name, "value": value})
}

// UpdateVariable replaces the value of the Actions variable name.
func (c *Client) UpdateVariable(ctx context.Context, owner, repo, name, value string) error {
	return c.send(ctx, http.MethodPatch, "repos/"+owner+"/"+repo+"/actions/variables/"+name, map[string]string{"name": name, "value": value})
}

// DeleteVariable deletes the Actions variable name.
func (c *Client) DeleteVariable(ctx context.Context, owner, repo, name string) error {
	return c.send(ctx, http.MethodDelete, "repos/"+owner+"/"+repo+"/actions/variables/"+name, nil)
}
