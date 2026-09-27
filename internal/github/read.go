package github

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/usadamasa/gh-manage/internal/config"
)

// Repo is an entry of the owner's repository list.
type Repo struct {
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
	Fork     bool   `json:"fork"`
}

// rulesetSummary is an entry of GET /repos/{owner}/{repo}/rulesets.
type rulesetSummary struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
}

// CurrentUser returns the login of the token's owner.
func (c *Client) CurrentUser(ctx context.Context) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if err := c.getJSON(ctx, "user", &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// ListOwnedRepos lists every repository the token's owner owns, archived and forks included.
func (c *Client) ListOwnedRepos(ctx context.Context) ([]Repo, error) {
	return getList[Repo](ctx, c, "user/repos?affiliation=owner&per_page=100")
}

// FetchSettings reads the live settings of owner/name in the shape of config.Settings.
// secret は値を読めないので名前だけ返し､from_env には同じ名前を入れる｡
func (c *Client) FetchSettings(ctx context.Context, owner, name string) (*config.Settings, error) {
	s := &config.Settings{}
	prefix := "repos/" + owner + "/" + name
	steps := []func() error{
		func() error { return c.readRepository(ctx, prefix, s) },
		func() error { return c.readRulesets(ctx, prefix, s) },
		func() error { return c.readVariables(ctx, prefix, s) },
		func() (err error) { s.Secrets, err = c.readSecretNames(ctx, prefix+"/actions/secrets"); return err },
		func() (err error) {
			s.DependabotSecrets, err = c.readSecretNames(ctx, prefix+"/dependabot/secrets")
			return err
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", owner, name, err)
		}
	}
	return s, nil
}

func (c *Client) readRepository(ctx context.Context, prefix string, s *config.Settings) error {
	var repo struct {
		config.Repository
		Topics []string `json:"topics"`
	}
	if err := c.getJSON(ctx, prefix, &repo); err != nil {
		return err
	}
	s.Repository = repo.Repository
	if len(repo.Topics) > 0 {
		s.Topics = repo.Topics
	}
	return nil
}

func (c *Client) readRulesets(ctx context.Context, prefix string, s *config.Settings) error {
	list, err := getList[rulesetSummary](ctx, c, prefix+"/rulesets?per_page=100")
	if err != nil {
		return err
	}
	for _, rs := range list {
		// organization などの上位から継承した ruleset はこのリポジトリでは管理しない
		if rs.SourceType != "Repository" {
			continue
		}
		var detail config.Ruleset
		if err := c.getJSON(ctx, fmt.Sprintf("%s/rulesets/%d", prefix, rs.ID), &detail); err != nil {
			return err
		}
		if s.Rulesets == nil {
			s.Rulesets = map[string]config.Ruleset{}
		}
		s.Rulesets[rs.Name] = detail
	}
	return nil
}

func (c *Client) readVariables(ctx context.Context, prefix string, s *config.Settings) error {
	// variables の per_page の上限は 30
	return c.getPages(ctx, prefix+"/actions/variables?per_page=30", func(body []byte) error {
		var page struct {
			Variables []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"variables"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		for _, v := range page.Variables {
			if s.Variables == nil {
				s.Variables = map[string]string{}
			}
			s.Variables[v.Name] = v.Value
		}
		return nil
	})
}

func (c *Client) readSecretNames(ctx context.Context, path string) (config.Secrets, error) {
	var out config.Secrets
	err := c.getPages(ctx, path+"?per_page=100", func(body []byte) error {
		var page struct {
			Secrets []struct {
				Name string `json:"name"`
			} `json:"secrets"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		for _, sec := range page.Secrets {
			if out == nil {
				out = config.Secrets{}
			}
			out[sec.Name] = config.Secret{FromEnv: sec.Name}
		}
		return nil
	})
	return out, err
}
