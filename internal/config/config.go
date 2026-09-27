package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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
func Load(dir string) (*Config, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open settings: %w", err)
	}
	defer func() { _ = root.Close() }()
	fsys := root.FS()

	base, err := loadFile(fsys, dir, "base.yaml")
	if err != nil {
		return nil, err
	}
	cfg := &Config{base: base, overlays: map[string]*yaml.Node{}}

	names, err := fs.Glob(fsys, "repos/*.yaml")
	if err != nil {
		return nil, fmt.Errorf("list overlays: %w", err)
	}
	for _, name := range names {
		overlay, err := loadFile(fsys, dir, name)
		if err != nil {
			return nil, err
		}
		cfg.overlays[strings.TrimSuffix(path.Base(name), ".yaml")] = overlay
	}
	return cfg, nil
}

// loadFile reads a YAML map and checks it against the schema.
func loadFile(fsys fs.FS, dir, name string) (*yaml.Node, error) {
	display := filepath.Join(dir, filepath.FromSlash(name))
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", display, err)
	}
	if err := decodeStrict(data, &Settings{}); err != nil {
		return nil, fmt.Errorf("%s: %w", display, err)
	}
	n, err := parseMapping(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", display, err)
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
