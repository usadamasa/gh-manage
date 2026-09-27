package config

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Settings is the desired state of one repository: base.yaml merged with repos/<name>.yaml.
type Settings struct {
	Prune             Prune              `yaml:"prune"`
	Repository        Repository         `yaml:"repository,omitempty"`
	Topics            []string           `yaml:"topics,omitempty"`
	Rulesets          map[string]Ruleset `yaml:"rulesets,omitempty"`
	Variables         map[string]string  `yaml:"variables,omitempty"`
	Secrets           Secrets            `yaml:"secrets,omitempty"`
	DependabotSecrets Secrets            `yaml:"dependabot_secrets,omitempty"`
}

// Prune decides, per kind, whether resources that are not declared are deleted. Default false.
type Prune struct {
	Rulesets          bool `yaml:"rulesets"`
	Variables         bool `yaml:"variables"`
	Secrets           bool `yaml:"secrets"`
	DependabotSecrets bool `yaml:"dependabot_secrets"`
}

// Repository holds the fields of PATCH /repos/{owner}/{repo}.
// nil は「宣言していない」を表し､plan で比較しない｡
// name は rename を防ぐため受け付けない｡
type Repository struct {
	Description              *string `yaml:"description,omitempty"`
	Homepage                 *string `yaml:"homepage,omitempty"`
	Visibility               *string `yaml:"visibility,omitempty"`
	HasIssues                *bool   `yaml:"has_issues,omitempty"`
	HasProjects              *bool   `yaml:"has_projects,omitempty"`
	HasWiki                  *bool   `yaml:"has_wiki,omitempty"`
	HasDiscussions           *bool   `yaml:"has_discussions,omitempty"`
	IsTemplate               *bool   `yaml:"is_template,omitempty"`
	DefaultBranch            *string `yaml:"default_branch,omitempty"`
	AllowSquashMerge         *bool   `yaml:"allow_squash_merge,omitempty"`
	AllowMergeCommit         *bool   `yaml:"allow_merge_commit,omitempty"`
	AllowRebaseMerge         *bool   `yaml:"allow_rebase_merge,omitempty"`
	AllowAutoMerge           *bool   `yaml:"allow_auto_merge,omitempty"`
	DeleteBranchOnMerge      *bool   `yaml:"delete_branch_on_merge,omitempty"`
	AllowUpdateBranch        *bool   `yaml:"allow_update_branch,omitempty"`
	AllowForking             *bool   `yaml:"allow_forking,omitempty"`
	WebCommitSignoffRequired *bool   `yaml:"web_commit_signoff_required,omitempty"`
	SquashMergeCommitTitle   *string `yaml:"squash_merge_commit_title,omitempty"`
	SquashMergeCommitMessage *string `yaml:"squash_merge_commit_message,omitempty"`
	MergeCommitTitle         *string `yaml:"merge_commit_title,omitempty"`
	MergeCommitMessage       *string `yaml:"merge_commit_message,omitempty"`
}

// Ruleset is a repository ruleset. The map key in Settings.Rulesets is its name.
type Ruleset struct {
	Target       string        `yaml:"target"`
	Enforcement  string        `yaml:"enforcement"`
	BypassActors []BypassActor `yaml:"bypass_actors,omitempty"`
	Conditions   *Conditions   `yaml:"conditions,omitempty"`
	Rules        []Rule        `yaml:"rules,omitempty"`
}

// BypassActor is an entry of bypass_actors in the rulesets API.
type BypassActor struct {
	ActorID    *int64 `yaml:"actor_id,omitempty"`
	ActorType  string `yaml:"actor_type"`
	BypassMode string `yaml:"bypass_mode,omitempty"`
}

// Conditions is the conditions object of the rulesets API.
type Conditions struct {
	RefName *RefName `yaml:"ref_name,omitempty"`
}

// RefName is conditions.ref_name of the rulesets API.
type RefName struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// Rule is an entry of rules[] in the rulesets API. parameters はそのまま API に渡す｡
type Rule struct {
	Type       string         `yaml:"type"`
	Parameters map[string]any `yaml:"parameters,omitempty"`
}

// Secret is a secret whose value is read from an environment variable at apply time.
type Secret struct {
	FromEnv string `yaml:"from_env"`
}

// Secrets maps a secret name to where its value comes from.
// 平文の値を YAML に書けないよう､from_env だけを受け付ける｡
type Secrets map[string]Secret

// UnmarshalYAML accepts only `NAME: {from_env: ENV}` (or `NAME: null` in an overlay).
func (s *Secrets) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: secrets must be a map", node.Line)
	}
	out := make(Secrets, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		if val.ShortTag() == "!!null" {
			continue
		}
		secret, err := decodeSecret(key.Value, val)
		if err != nil {
			return err
		}
		out[key.Value] = secret
	}
	*s = out
	return nil
}

func decodeSecret(name string, val *yaml.Node) (Secret, error) {
	if val.Kind != yaml.MappingNode {
		return Secret{}, fmt.Errorf("line %d: secret %s: 値は平文で書けない｡`from_env: <環境変数名>` で指定する", val.Line, name)
	}
	var secret Secret
	for i := 0; i+1 < len(val.Content); i += 2 {
		k, v := val.Content[i], val.Content[i+1]
		if k.Value != "from_env" {
			return Secret{}, fmt.Errorf("line %d: secret %s: field %s は使えない｡`from_env` だけを受け付ける", k.Line, name, k.Value)
		}
		secret.FromEnv = v.Value
	}
	return secret, nil
}

var (
	envNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	visibilities      = []string{"public", "private"}
	rulesetTargets    = []string{"branch", "tag", "push"}
	rulesetEnforcemts = []string{"active", "evaluate", "disabled"}
)

// validate checks values that the YAML schema alone cannot.
func (s *Settings) validate() error {
	var errs []error
	errs = append(errs, validateRepository(s.Repository)...)
	errs = append(errs, validateVariables(s.Variables)...)
	errs = append(errs, validateSecrets("secrets", s.Secrets)...)
	errs = append(errs, validateSecrets("dependabot_secrets", s.DependabotSecrets)...)
	errs = append(errs, validateRulesets(s.Rulesets)...)
	return errors.Join(errs...)
}

func validateRepository(r Repository) []error {
	if v := r.Visibility; v != nil && !slices.Contains(visibilities, *v) {
		return []error{fmt.Errorf("repository.visibility: %q は %v のどれかにする", *v, visibilities)}
	}
	return nil
}

func validateVariables(vars map[string]string) []error {
	var errs []error
	for _, name := range sortedKeys(vars) {
		errs = append(errs, validateActionsName("variables", name))
	}
	return errs
}

func validateRulesets(rulesets map[string]Ruleset) []error {
	var errs []error
	for _, name := range sortedKeys(rulesets) {
		errs = append(errs, rulesets[name].validate("rulesets."+name)...)
	}
	return errs
}

func validateSecrets(kind string, secrets Secrets) []error {
	var errs []error
	for _, name := range sortedKeys(secrets) {
		errs = append(errs, validateActionsName(kind, name))
		if env := secrets[name].FromEnv; !envNamePattern.MatchString(env) {
			errs = append(errs, fmt.Errorf("%s.%s.from_env: %q は環境変数名として使えない", kind, name, env))
		}
	}
	return errs
}

// validateActionsName checks the naming rules of Actions secrets and variables.
func validateActionsName(kind, name string) error {
	if !envNamePattern.MatchString(name) || strings.HasPrefix(strings.ToUpper(name), "GITHUB_") {
		return fmt.Errorf("%s.%s: 名前は英数字と _ で､数字や GITHUB_ で始められない", kind, name)
	}
	return nil
}

func (r Ruleset) validate(path string) []error {
	var errs []error
	if !slices.Contains(rulesetTargets, r.Target) {
		errs = append(errs, fmt.Errorf("%s.target: %q は %v のどれかにする", path, r.Target, rulesetTargets))
	}
	if !slices.Contains(rulesetEnforcemts, r.Enforcement) {
		errs = append(errs, fmt.Errorf("%s.enforcement: %q は %v のどれかにする", path, r.Enforcement, rulesetEnforcemts))
	}
	for i, rule := range r.Rules {
		if rule.Type == "" {
			errs = append(errs, fmt.Errorf("%s.rules[%d].type: 必須", path, i))
		}
	}
	return errs
}

// Encode encodes v (Settings or a map of them) as YAML with 2-space indentation.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	return buf.Bytes(), nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
