package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

// settings/schema.json はエディタ向けの写しで、正は decodeStrict と validate()。
// ここのテストで両者のずれを検出する。
const (
	schemaPath   = "../../settings/schema.json"
	settingsPath = "../../settings"
)

func readSchemaDoc(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse %s: %v", schemaPath, err)
	}
	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("%s: top level must be an object", schemaPath)
	}
	return m
}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", readSchemaDoc(t)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		t.Fatalf("compile %s: %v", schemaPath, err)
	}
	return sch
}

// yamlToJSON converts a YAML document into the value jsonschema validates. 空の document は null になる。
func yamlToJSON(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	out, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSchema_AcceptsSettings(t *testing.T) {
	sch := compileSchema(t)
	overlays, err := filepath.Glob(filepath.Join(settingsPath, "repos", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	paths := append([]string{filepath.Join(settingsPath, "base.yaml")}, overlays...)
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path) // #nosec G304 -- リポジトリの settings を読むテスト
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(yamlToJSON(t, data)); err != nil {
				t.Errorf("schema rejects %s: %v", path, err)
			}
		})
	}
}

// TestSchema_AgreesWithGo checks the schema and Go on the same base.yaml.
// schema は 1 ファイル単位でしか見られないので、merge 後にしか決まらない検査 (target の必須など) は schema 側で通す。
func TestSchema_AgreesWithGo(t *testing.T) {
	tests := []struct {
		name          string
		base          string
		wantSchemaErr bool
		wantGoErr     bool
	}{
		{name: "testBase", base: testBase},
		{name: "空の document", base: ""},
		{name: "空の map", base: "{}\n"},
		{name: "未知のキー", base: "foo: 1\n", wantSchemaErr: true, wantGoErr: true},
		{name: "repository の未知のキー", base: "repository:\n  name: x\n", wantSchemaErr: true, wantGoErr: true},
		{name: "visibility の enum", base: "repository:\n  visibility: internal\n", wantSchemaErr: true, wantGoErr: true},
		{name: "bool に文字列", base: "repository:\n  has_issues: maybe\n", wantSchemaErr: true, wantGoErr: true},
		// yaml.v3 は bool の field に限って YAML 1.1 の yes / on を受ける。schema は true / false だけを勧める
		{name: "bool に yes", base: "repository:\n  has_issues: \"yes\"\n", wantSchemaErr: true},
		{name: "overlay の null で削除", base: "repository:\n  description: null\nrulesets: null\nsecrets:\n  X: null\n"},
		{name: "managed_topic", base: "managed_topic: gh-managed\n"},
		{name: "managed_topic の空文字", base: "managed_topic: \"\"\n"},
		{name: "managed_topic の pattern", base: "managed_topic: Gh_Managed\n", wantSchemaErr: true, wantGoErr: true},
		{name: "ruleset の target の enum", base: "rulesets:\n  r:\n    target: foo\n    enforcement: active\n", wantSchemaErr: true, wantGoErr: true},
		{name: "ruleset の enforcement の enum", base: "rulesets:\n  r:\n    target: branch\n    enforcement: on\n", wantSchemaErr: true, wantGoErr: true},
		{name: "ruleset の target 無しは merge 後に決まる", base: "rulesets:\n  r:\n    enforcement: active\n", wantGoErr: true},
		{name: "rule の type 無し", base: "rulesets:\n  r:\n    target: branch\n    enforcement: active\n    rules:\n      - parameters: {}\n", wantSchemaErr: true, wantGoErr: true},
		{name: "rule の parameters は自由", base: "rulesets:\n  r:\n    target: branch\n    enforcement: active\n    rules:\n      - type: x\n        parameters:\n          a: [1, null]\n"},
		{name: "actor_id に文字列", base: "rulesets:\n  r:\n    target: branch\n    enforcement: active\n    bypass_actors:\n      - actor_id: abc\n        actor_type: Team\n", wantSchemaErr: true, wantGoErr: true},
		{name: "variable の平文と from_env", base: "variables:\n  A: x\n  B: 1\n  C:\n    from_env: C_ENV\n"},
		{name: "variable の GITHUB_ 接頭辞", base: "variables:\n  github_token: x\n", wantSchemaErr: true, wantGoErr: true},
		{name: "variable の名前が数字で始まる", base: "variables:\n  1A: x\n", wantSchemaErr: true, wantGoErr: true},
		{name: "variable の未知のフィールド", base: "variables:\n  A:\n    value: x\n", wantSchemaErr: true, wantGoErr: true},
		{name: "variable の from_env が空", base: "variables:\n  A:\n    from_env: \"\"\n", wantSchemaErr: true, wantGoErr: true},
		{name: "variable の値が list", base: "variables:\n  A: [x]\n", wantSchemaErr: true, wantGoErr: true},
		{name: "secret の平文", base: "secrets:\n  A: x\n", wantSchemaErr: true, wantGoErr: true},
		{name: "secret の from_env 無し", base: "secrets:\n  A: {}\n", wantSchemaErr: true, wantGoErr: true},
		{name: "secret の from_env の pattern", base: "dependabot_secrets:\n  A:\n    from_env: 1BAD\n", wantSchemaErr: true, wantGoErr: true},
		{name: "secret の GITHUB_ 接頭辞", base: "secrets:\n  GITHUB_X:\n    from_env: X\n", wantSchemaErr: true, wantGoErr: true},
		{name: "data", base: "data:\n  rulesets: [legacy main]\n  variables: [V]\n  secrets: [S]\n  dependabot_secrets: [D]\n"},
		{name: "data の null で削除", base: "data:\n  secrets: null\n"},
		{name: "data の未知のキー", base: "data:\n  topics: [x]\n", wantSchemaErr: true, wantGoErr: true},
		{name: "data の値が map", base: "data:\n  secrets:\n    S: {}\n", wantSchemaErr: true, wantGoErr: true},
		{name: "data の secret の GITHUB_ 接頭辞", base: "data:\n  secrets: [GITHUB_X]\n", wantSchemaErr: true, wantGoErr: true},
		{name: "data の variable の名前が数字で始まる", base: "data:\n  variables: [1A]\n", wantSchemaErr: true, wantGoErr: true},
		{name: "data の重複", base: "data:\n  secrets: [S, S]\n", wantSchemaErr: true, wantGoErr: true},
		// 別のセクションとの衝突は merge 後にしか決まらない
		{name: "data と secrets の両方に同じ名前", base: "secrets:\n  S:\n    from_env: S\ndata:\n  secrets: [S]\n", wantGoErr: true},
		{name: "data と rulesets の両方に同じ名前", base: "rulesets:\n  r:\n    target: branch\n    enforcement: active\ndata:\n  rulesets: [r]\n", wantGoErr: true},
	}
	sch := compileSchema(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaErr := sch.Validate(yamlToJSON(t, []byte(tt.base)))
			if (schemaErr != nil) != tt.wantSchemaErr {
				t.Errorf("schema: error = %v, wantErr %v", schemaErr, tt.wantSchemaErr)
			}

			dir := writeSettings(t, tt.base, map[string]string{"r": "{}\n"})
			cfg, goErr := Load(dir)
			if goErr == nil {
				_, goErr = cfg.Render("r")
			}
			if (goErr != nil) != tt.wantGoErr {
				t.Errorf("go: error = %v, wantErr %v", goErr, tt.wantGoErr)
			}
		})
	}
}

// lookup follows a JSON pointer such as "/definitions/ruleset" from the schema root.
func lookup(t *testing.T, doc map[string]any, pointer string) map[string]any {
	t.Helper()
	cur := doc
	for key := range strings.SplitSeq(strings.TrimPrefix(pointer, "/"), "/") {
		if key == "" {
			continue
		}
		next, ok := cur[key].(map[string]any)
		if !ok {
			t.Fatalf("schema has no object at %s (missing %q)", pointer, key)
		}
		cur = next
	}
	return cur
}

func TestSchema_PropertiesMatchStructs(t *testing.T) {
	tests := []struct {
		typ     reflect.Type
		pointer string
	}{
		{reflect.TypeFor[Settings](), "/"},
		{reflect.TypeFor[Prune](), "/definitions/prune"},
		{reflect.TypeFor[Data](), "/definitions/data"},
		{reflect.TypeFor[Repository](), "/definitions/repository"},
		{reflect.TypeFor[Ruleset](), "/definitions/ruleset"},
		{reflect.TypeFor[BypassActor](), "/definitions/bypass_actor"},
		{reflect.TypeFor[Conditions](), "/definitions/conditions"},
		{reflect.TypeFor[RefName](), "/definitions/ref_name"},
		{reflect.TypeFor[Rule](), "/definitions/rule"},
		{reflect.TypeFor[Secret](), "/definitions/secret"},
	}
	doc := readSchemaDoc(t)
	for _, tt := range tests {
		t.Run(tt.typ.Name(), func(t *testing.T) {
			var want []string
			for f := range tt.typ.Fields() {
				want = append(want, strings.Split(f.Tag.Get("yaml"), ",")[0])
			}
			obj := lookup(t, doc, tt.pointer)
			props, _ := obj["properties"].(map[string]any)
			var got []string
			for k := range props {
				got = append(got, k)
			}
			slices.Sort(want)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("%s properties = %v, want %v (yaml tags of %s)", tt.pointer, got, want, tt.typ)
			}
			if obj["additionalProperties"] != false {
				t.Errorf("%s: additionalProperties must be false (decodeStrict rejects unknown keys)", tt.pointer)
			}
		})
	}
}

func TestSchema_EnumsMatchGo(t *testing.T) {
	tests := []struct {
		pointer string
		want    []string
	}{
		{"/definitions/repository/properties/visibility", visibilities},
		{"/definitions/ruleset/properties/target", rulesetTargets},
		{"/definitions/ruleset/properties/enforcement", rulesetEnforcemts},
	}
	doc := readSchemaDoc(t)
	for _, tt := range tests {
		t.Run(tt.pointer, func(t *testing.T) {
			raw, _ := lookup(t, doc, tt.pointer)["enum"].([]any)
			var got []string
			for _, v := range raw {
				if s, ok := v.(string); ok {
					got = append(got, s)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("%s enum = %v, want %v", tt.pointer, got, tt.want)
			}
		})
	}
}

func TestSchema_PatternsMatchGo(t *testing.T) {
	tests := []struct {
		pointer string
		want    string
	}{
		{"/definitions/env_name", envNamePattern.String()},
		{"/definitions/topic", topicPattern.String()},
	}
	doc := readSchemaDoc(t)
	for _, tt := range tests {
		t.Run(tt.pointer, func(t *testing.T) {
			if got := lookup(t, doc, tt.pointer)["pattern"]; got != tt.want {
				t.Errorf("%s pattern = %v, want %q", tt.pointer, got, tt.want)
			}
		})
	}
}
