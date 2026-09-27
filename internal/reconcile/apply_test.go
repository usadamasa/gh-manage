package reconcile

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/usadamasa/gh-manage/internal/config"
)

// fakeWriter applies writes to an in-memory live state, the way GitHub would.
type fakeWriter struct {
	live  *config.Settings
	calls []string
}

func (f *fakeWriter) CreateRepository(_ context.Context, name string, r config.Repository) error {
	f.calls = append(f.calls, "create "+name)
	f.live = &config.Settings{Repository: config.Repository{Visibility: r.Visibility, Description: r.Description}}
	return nil
}

func (f *fakeWriter) UpdateRepository(_ context.Context, _, _ string, fields map[string]any) error {
	b, _ := json.Marshal(fields)
	f.calls = append(f.calls, "patch "+string(b))
	return json.Unmarshal(b, &f.live.Repository)
}

func (f *fakeWriter) SetTopics(_ context.Context, _, _ string, topics []string) error {
	f.calls = append(f.calls, "topics "+strings.Join(topics, ","))
	f.live.Topics = topics
	return nil
}

func (f *fakeWriter) UpsertRuleset(_ context.Context, _, _, name string, rs config.Ruleset) error {
	f.calls = append(f.calls, "upsert ruleset "+name)
	if f.live.Rulesets == nil {
		f.live.Rulesets = map[string]config.Ruleset{}
	}
	f.live.Rulesets[name] = rs
	return nil
}

func (f *fakeWriter) DeleteRuleset(_ context.Context, _, _, name string) error {
	f.calls = append(f.calls, "delete ruleset "+name)
	delete(f.live.Rulesets, name)
	return nil
}

// applied reports the changes Apply handles in this step (repository, topics, rulesets).
func applied(p RepoPlan) []Change {
	var out []Change
	for _, c := range p.Changes {
		switch c.Kind {
		case "repository", "topics", "ruleset":
			out = append(out, c)
		}
	}
	return out
}

// TestApply_Converges applies every golden plan and checks that planning again finds nothing.
func TestApply_Converges(t *testing.T) {
	for _, name := range []string{"repository", "rulesets", "prune", "topics", "create_repo", "no_change"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata/plan", name)
			desired := readSettings(t, filepath.Join(dir, "desired.yaml"))
			w := &fakeWriter{live: readSettings(t, filepath.Join(dir, "live.yaml"))}

			if err := Apply(context.Background(), w, "o", Plan(name, desired, w.live), desired); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if rest := applied(Plan(name, desired, w.live)); len(rest) != 0 {
				t.Errorf("plan after apply = %+v, want none (calls %q)", rest, w.calls)
			}
		})
	}
}

func TestApply_Calls(t *testing.T) {
	desired := &config.Settings{
		Repository: config.Repository{HasWiki: ptr(false), Visibility: ptr("public")},
		Rulesets:   map[string]config.Ruleset{"main": {Target: "branch", Enforcement: "active"}},
	}
	tests := []struct {
		name string
		live *config.Settings
		want []string
	}{
		{
			name: "変わったキーだけ PATCH し､ruleset は名前ごとに 1 回",
			live: &config.Settings{
				Repository: config.Repository{HasWiki: ptr(true), Visibility: ptr("public"), HasIssues: ptr(true)},
				Rulesets:   map[string]config.Ruleset{"main": {Target: "tag", Enforcement: "disabled"}},
			},
			want: []string{`patch {"has_wiki":false}`, "upsert ruleset main"},
		},
		{
			name: "無いリポジトリは作ってから宣言した設定を全部入れる",
			live: nil,
			want: []string{"create r", `patch {"has_wiki":false,"visibility":"public"}`, "upsert ruleset main"},
		},
		{
			name: "差分が無ければ何も呼ばない",
			live: &config.Settings{
				Repository: config.Repository{HasWiki: ptr(false), Visibility: ptr("public")},
				Rulesets:   map[string]config.Ruleset{"main": {Target: "branch", Enforcement: "active"}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &fakeWriter{live: tt.live}
			if err := Apply(context.Background(), w, "o", Plan("r", desired, tt.live), desired); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if !reflect.DeepEqual(w.calls, tt.want) {
				t.Errorf("calls = %q, want %q", w.calls, tt.want)
			}
		})
	}
}

func TestApply_SkipDoesNothing(t *testing.T) {
	w := &fakeWriter{}
	if err := Apply(context.Background(), w, "o", Skip("r", "archived"), &config.Settings{}); err != nil {
		t.Fatal(err)
	}
	if len(w.calls) != 0 {
		t.Errorf("calls = %q, want none", w.calls)
	}
}

func TestCheckPublish(t *testing.T) {
	private := &config.Settings{Repository: config.Repository{Visibility: ptr("private")}}
	public := &config.Settings{Repository: config.Repository{Visibility: ptr("public")}}
	tests := []struct {
		name    string
		plans   []RepoPlan
		wantErr bool
	}{
		{"private → public は拒否", []RepoPlan{Plan("a", public, private)}, true},
		{"public → private は通す", []RepoPlan{Plan("a", private, public)}, false},
		{"public で新規作成は通す", []RepoPlan{Plan("a", public, nil)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPublish(tt.plans)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPublish() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "a") {
				t.Errorf("error %q should name the repository", err)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
