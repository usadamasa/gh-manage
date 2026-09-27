package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config holds the raw base and overlays read from a settings directory.
type Config struct {
	base     *yaml.Node
	overlays map[string]*yaml.Node
}

// Load reads <dir>/base.yaml and <dir>/repos/*.yaml.
// A file under repos/ means the repository is managed. Each file is checked
// against the schema so that a typo fails here, not at apply.
//
// os.Root は使わない｡Go 1.24 の os.Root には未修正の脆弱性があり govulncheck が落ちる｡
// dir は利用者が指定する settings ディレクトリそのものなので､閉じ込める必要も無い｡
func Load(dir string) (*Config, error) {
	base, err := loadFile(filepath.Join(dir, "base.yaml"))
	if err != nil {
		return nil, err
	}
	cfg := &Config{base: base, overlays: map[string]*yaml.Node{}}

	paths, err := filepath.Glob(filepath.Join(dir, "repos", "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("list overlays: %w", err)
	}
	for _, path := range paths {
		overlay, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		cfg.overlays[strings.TrimSuffix(filepath.Base(path), ".yaml")] = overlay
	}
	return cfg, nil
}

// loadFile reads a YAML map and checks it against the schema.
func loadFile(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path は利用者が指定した settings ディレクトリの中
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := decodeStrict(data, &Settings{}); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	n, err := parseMapping(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return n, nil
}

// decodeStrict decodes data into out, rejecting keys that are not in the schema.
// An empty document is accepted.
func decodeStrict(data []byte, out *Settings) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err //nolint:wrapcheck // 呼び出し側でファイル名を付ける
	}
	return nil
}

// Names returns the managed repository names in sorted order.
func (c *Config) Names() []string {
	return sortedKeys(c.overlays)
}

// Render merges the overlay of the named repository into base and validates the result.
func (c *Config) Render(name string) (*Settings, error) {
	overlay, ok := c.overlays[name]
	if !ok {
		return nil, fmt.Errorf("repository %q is not managed: settings/repos/%s.yaml does not exist", name, name)
	}
	data, err := yaml.Marshal(merge(c.base, overlay))
	if err != nil {
		return nil, fmt.Errorf("%s: encode merged settings: %w", name, err)
	}
	var s Settings
	if err := decodeStrict(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &s, nil
}

// RenderAll renders every managed repository, keyed by name.
func (c *Config) RenderAll() (map[string]*Settings, error) {
	out := make(map[string]*Settings, len(c.overlays))
	var errs []error
	for _, name := range c.Names() {
		s, err := c.Render(name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out[name] = s
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}
