package github

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/usadamasa/gh-manage/internal/config"
)

func TestWrites(t *testing.T) {
	rulesetList := fakeResponse{body: `[{"id":7,"name":"main","source_type":"Repository"},{"id":9,"name":"org","source_type":"Organization"}]`}
	tests := []struct {
		name string
		do   func(c *Client) error
		want []string
	}{
		{
			name: "UpdateRepository は渡したキーだけを PATCH する",
			do: func(c *Client) error {
				return c.UpdateRepository(context.Background(), "o", "r", map[string]any{"has_wiki": false})
			},
			want: []string{`PATCH repos/o/r {"has_wiki":false}`},
		},
		{
			name: "CreateRepository は private と description を POST し auto_init する",
			do: func(c *Client) error {
				return c.CreateRepository(context.Background(), "r", config.Repository{Visibility: ptr("private"), Description: ptr("d"), HasWiki: ptr(false)})
			},
			want: []string{`POST user/repos {"auto_init":true,"description":"d","name":"r","private":true}`},
		},
		{
			name: "SetTopics は names を PUT する",
			do:   func(c *Client) error { return c.SetTopics(context.Background(), "o", "r", []string{"go", "cli"}) },
			want: []string{`PUT repos/o/r/topics {"names":["go","cli"]}`},
		},
		{
			name: "SetTopics は空でも空配列を送る",
			do:   func(c *Client) error { return c.SetTopics(context.Background(), "o", "r", nil) },
			want: []string{`PUT repos/o/r/topics {"names":[]}`},
		},
		{
			name: "UpsertRuleset は既存なら PUT",
			do: func(c *Client) error {
				return c.UpsertRuleset(context.Background(), "o", "r", "main", config.Ruleset{Target: "branch", Enforcement: "active", Rules: []config.Rule{{Type: "deletion"}}})
			},
			want: []string{`PUT repos/o/r/rulesets/7 {"target":"branch","enforcement":"active","rules":[{"type":"deletion"}],"name":"main"}`},
		},
		{
			name: "UpsertRuleset は無ければ POST (organization の同名は見ない)",
			do: func(c *Client) error {
				return c.UpsertRuleset(context.Background(), "o", "r", "org", config.Ruleset{Target: "tag", Enforcement: "active"})
			},
			want: []string{`POST repos/o/r/rulesets {"target":"tag","enforcement":"active","name":"org"}`},
		},
		{
			name: "DeleteRuleset は名前から id を引いて DELETE",
			do:   func(c *Client) error { return c.DeleteRuleset(context.Background(), "o", "r", "main") },
			want: []string{`DELETE repos/o/r/rulesets/7`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ft := newTestClient(t, map[string]fakeResponse{
				"GET repos/o/r/rulesets?per_page=100": rulesetList,
				"PATCH repos/o/r":                     {body: `{}`},
				"POST user/repos":                     {status: http.StatusCreated, body: `{}`},
				"PUT repos/o/r/topics":                {body: `{}`},
				"PUT repos/o/r/rulesets/7":            {body: `{}`},
				"POST repos/o/r/rulesets":             {status: http.StatusCreated, body: `{}`},
				"DELETE repos/o/r/rulesets/7":         {status: http.StatusNoContent},
			})
			if err := tt.do(c); err != nil {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(ft.calls, tt.want) {
				t.Errorf("calls =\n%q\nwant\n%q", ft.calls, tt.want)
			}
		})
	}
}

func TestDeleteRuleset_NotFound(t *testing.T) {
	c, _ := newTestClient(t, map[string]fakeResponse{"GET repos/o/r/rulesets?per_page=100": {body: `[]`}})
	if err := c.DeleteRuleset(context.Background(), "o", "r", "missing"); err == nil {
		t.Error("DeleteRuleset() error = nil, want error")
	}
}
