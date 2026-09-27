package github

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/usadamasa/gh-manage/internal/config"
)

// fakeResponse is a canned response for one "METHOD path?query" key.
type fakeResponse struct {
	status int
	body   string
	header map[string]string
}

// fakeTransport answers requests from a table instead of the network.
// httptest.NewServer は sandbox で bind できないので RoundTripper を差し替える｡
type fakeTransport struct {
	responses map[string]fakeResponse
	requests  []*http.Request
	// calls records "METHOD path body" of every request other than GET.
	calls []string
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req)
	if req.Method != http.MethodGet {
		var body []byte
		if req.Body != nil {
			body, _ = io.ReadAll(req.Body)
		}
		f.calls = append(f.calls, strings.TrimSpace(req.Method+" "+strings.TrimPrefix(req.URL.Path, "/")+" "+string(body)))
	}
	key := req.Method + " " + strings.TrimPrefix(req.URL.Path, "/")
	if req.URL.RawQuery != "" {
		key += "?" + req.URL.RawQuery
	}
	r, ok := f.responses[key]
	if !ok {
		r = fakeResponse{status: http.StatusNotFound, body: `{"message":"Not Found: ` + key + `"}`}
	}
	if r.status == 0 {
		r.status = http.StatusOK
	}
	h := http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}
	for k, v := range r.header {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: r.status,
		Status:     http.StatusText(r.status),
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Request:    req,
	}, nil
}

func newTestClient(t *testing.T, responses map[string]fakeResponse) (*Client, *fakeTransport) {
	t.Helper()
	ft := &fakeTransport{responses: responses}
	c, err := New(Options{Token: "test-token", Transport: ft})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return c, ft
}

func ptr[T any](v T) *T { return &v }

func TestNew_Token(t *testing.T) {
	tests := []struct {
		name  string
		opt   string
		env   string
		wantH string
	}{
		{name: "Options.Token を使う", opt: "opt-token", env: "env-token", wantH: "token opt-token"},
		{name: "無ければ GH_TOKEN を使う", opt: "", env: "env-token", wantH: "token env-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_TOKEN", tt.env)
			ft := &fakeTransport{responses: map[string]fakeResponse{"GET user": {body: `{"login":"octo"}`}}}
			c, err := New(Options{Token: tt.opt, Transport: ft})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if _, err := c.CurrentUser(context.Background()); err != nil {
				t.Fatalf("CurrentUser() error = %v", err)
			}
			if got := ft.requests[0].Header.Get("Authorization"); got != tt.wantH {
				t.Errorf("Authorization = %q, want %q", got, tt.wantH)
			}
		})
	}
}

func TestCurrentUser(t *testing.T) {
	c, _ := newTestClient(t, map[string]fakeResponse{"GET user": {body: `{"login":"usadamasa"}`}})
	got, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser() error = %v", err)
	}
	if got != "usadamasa" {
		t.Errorf("CurrentUser() = %q, want usadamasa", got)
	}
}

func TestListOwnedRepos_Paginates(t *testing.T) {
	c, _ := newTestClient(t, map[string]fakeResponse{
		"GET user/repos?affiliation=owner&per_page=100": {
			body:   `[{"name":"a","archived":false,"fork":false},{"name":"b","archived":true,"fork":false}]`,
			header: map[string]string{"Link": `<https://api.github.com/user/repos?affiliation=owner&per_page=100&page=2>; rel="next", <https://api.github.com/user/repos?affiliation=owner&per_page=100&page=2>; rel="last"`},
		},
		"GET user/repos?affiliation=owner&per_page=100&page=2": {
			body: `[{"name":"c","archived":false,"fork":true}]`,
		},
	})
	got, err := c.ListOwnedRepos(context.Background())
	if err != nil {
		t.Fatalf("ListOwnedRepos() error = %v", err)
	}
	want := []Repo{{Name: "a"}, {Name: "b", Archived: true}, {Name: "c", Fork: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListOwnedRepos() = %+v, want %+v", got, want)
	}
}

func TestFetchSettings(t *testing.T) {
	c, _ := newTestClient(t, map[string]fakeResponse{
		"GET repos/o/r": {
			body: `{"name":"r","description":"desc","homepage":null,"visibility":"public","private":false,
				"has_issues":true,"has_wiki":false,"delete_branch_on_merge":true,"topics":["go","cli"]}`,
			header: map[string]string{"X-RateLimit-Remaining": "4999"},
		},
		"GET repos/o/r/rulesets?per_page=100": {
			body: `[{"id":1,"name":"main","source_type":"Repository"},{"id":2,"name":"org","source_type":"Organization"}]`,
		},
		"GET repos/o/r/rulesets/1": {
			body: `{"id":1,"name":"main","target":"branch","enforcement":"active","source_type":"Repository",
				"bypass_actors":[{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"}],
				"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},
				"rules":[{"type":"deletion"},{"type":"pull_request","parameters":{"required_approving_review_count":0}}]}`,
		},
		"GET repos/o/r/actions/variables?per_page=30": {
			body: `{"total_count":1,"variables":[{"name":"GO_VERSION","value":"1.25"}]}`,
		},
		"GET repos/o/r/actions/secrets?per_page=100": {
			body: `{"total_count":1,"secrets":[{"name":"TAGPR_PRIVATE_KEY","created_at":"2026-01-01T00:00:00Z"}]}`,
		},
		"GET repos/o/r/dependabot/secrets?per_page=100": {
			body:   `{"total_count":0,"secrets":[]}`,
			header: map[string]string{"X-RateLimit-Remaining": "4990"},
		},
	})
	got, err := c.FetchSettings(context.Background(), "o", "r")
	if err != nil {
		t.Fatalf("FetchSettings() error = %v", err)
	}
	want := &config.Settings{
		Repository: config.Repository{
			Description:         ptr("desc"),
			Visibility:          ptr("public"),
			HasIssues:           ptr(true),
			HasWiki:             ptr(false),
			DeleteBranchOnMerge: ptr(true),
		},
		Topics: []string{"go", "cli"},
		Rulesets: map[string]config.Ruleset{
			"main": {
				Target:       "branch",
				Enforcement:  "active",
				BypassActors: []config.BypassActor{{ActorID: ptr(int64(5)), ActorType: "RepositoryRole", BypassMode: "always"}},
				Conditions: &config.Conditions{RefName: &config.RefName{
					Include: []string{"~DEFAULT_BRANCH"},
					Exclude: []string{},
				}},
				Rules: []config.Rule{
					{Type: "deletion"},
					{Type: "pull_request", Parameters: map[string]any{"required_approving_review_count": float64(0)}},
				},
			},
		},
		Variables: config.Variables{"GO_VERSION": {Value: "1.25"}},
		Secrets:   config.Secrets{"TAGPR_PRIVATE_KEY": {FromEnv: "TAGPR_PRIVATE_KEY"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FetchSettings() =\n%#v\nwant\n%#v", got, want)
	}
	if got := c.RateLimitRemaining(); got != "4990" {
		t.Errorf("RateLimitRemaining() = %q, want 4990", got)
	}
}

func TestFetchSettings_Error(t *testing.T) {
	c, _ := newTestClient(t, map[string]fakeResponse{})
	_, err := c.FetchSettings(context.Background(), "o", "missing")
	if err == nil || !strings.Contains(err.Error(), "o/missing") {
		t.Errorf("FetchSettings() error = %v, want error naming o/missing", err)
	}
}
