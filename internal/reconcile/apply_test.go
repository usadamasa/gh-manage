package reconcile

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/usadamasa/gh-manage/internal/config"
)

// fakeGitHub applies actions to an in-memory live state, the way GitHub would.
type fakeGitHub struct {
	live  *config.Settings
	calls []string
}

func (f *fakeGitHub) run(t *testing.T, acts []Action) {
	t.Helper()
	for _, a := range acts {
		switch a.Op {
		case ActCreateRepository:
			f.calls = append(f.calls, "create")
			f.live = &config.Settings{Repository: config.Repository{Visibility: a.Repository.Visibility, Description: a.Repository.Description}}
		case ActUpdateRepository:
			b, _ := json.Marshal(a.Fields)
			f.calls = append(f.calls, "patch "+string(b))
			if err := json.Unmarshal(b, &f.live.Repository); err != nil {
				t.Fatal(err)
			}
		case ActSetTopics:
			f.calls = append(f.calls, "topics "+strings.Join(a.Topics, ","))
			f.live.Topics = a.Topics
		case ActUpsertRuleset:
			f.calls = append(f.calls, "upsert ruleset "+a.Name)
			if f.live.Rulesets == nil {
				f.live.Rulesets = map[string]config.Ruleset{}
			}
			f.live.Rulesets[a.Name] = a.Ruleset
		case ActDeleteRuleset:
			f.calls = append(f.calls, "delete ruleset "+a.Name)
			delete(f.live.Rulesets, a.Name)
		}
	}
}

func actions(t *testing.T, p RepoPlan, desired *config.Settings) []Action {
	t.Helper()
	acts, err := Actions(p, desired)
	if err != nil {
		t.Fatalf("Actions() error = %v", err)
	}
	return acts
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
			w := &fakeGitHub{live: readSettings(t, filepath.Join(dir, "live.yaml"))}
			w.run(t, actions(t, Plan(name, desired, w.live), desired))
			if rest := applied(Plan(name, desired, w.live)); len(rest) != 0 {
				t.Errorf("plan after apply = %+v, want none (calls %q)", rest, w.calls)
			}
		})
	}
}

func TestActions_Calls(t *testing.T) {
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
			want: []string{"create", `patch {"has_wiki":false,"visibility":"public"}`, "upsert ruleset main"},
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
			w := &fakeGitHub{live: tt.live}
			w.run(t, actions(t, Plan("r", desired, tt.live), desired))
			if !reflect.DeepEqual(w.calls, tt.want) {
				t.Errorf("calls = %q, want %q", w.calls, tt.want)
			}
		})
	}
}

func TestActions_Skip(t *testing.T) {
	if acts := actions(t, Skip("r", "archived"), &config.Settings{}); len(acts) != 0 {
		t.Errorf("Actions() = %+v, want none", acts)
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
