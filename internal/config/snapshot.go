package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// EncodeOverlay encodes live settings as an overlay file. prune は live に無いので書かない｡
func EncodeOverlay(live *Settings) ([]byte, error) {
	n, err := overlayNode(live)
	if err != nil {
		return nil, err
	}
	return Encode(n)
}

// Minimize encodes live settings as an overlay that keeps only what differs from base.
// base にあって live に無いキーは null にするので､render すると live に戻る｡
func (c *Config) Minimize(live *Settings) ([]byte, error) {
	n, err := overlayNode(live)
	if err != nil {
		return nil, err
	}
	base := merge(c.base, nil)
	removeKey(base, "prune")
	return Encode(diff(base, n))
}

func overlayNode(live *Settings) (*yaml.Node, error) {
	data, err := yaml.Marshal(live)
	if err != nil {
		return nil, fmt.Errorf("encode live settings: %w", err)
	}
	n, err := parseMapping(data)
	if err != nil {
		return nil, fmt.Errorf("encode live settings: %w", err)
	}
	removeKey(n, "prune")
	return n, nil
}

// diff returns the overlay that turns base into live.
func diff(base, live *yaml.Node) *yaml.Node {
	baseValues := map[string]*yaml.Node{}
	for k, v := range pairs(base) {
		baseValues[k.Value] = v
	}
	liveKeys := map[string]bool{}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for k, v := range pairs(live) {
		liveKeys[k.Value] = true
		bv, ok := baseValues[k.Value]
		switch {
		case !ok:
			out.Content = append(out.Content, k, v)
		case bv.Kind == yaml.MappingNode && v.Kind == yaml.MappingNode:
			if d := diff(bv, v); len(d.Content) > 0 {
				out.Content = append(out.Content, k, d)
			}
		case !nodeEqual(bv, v):
			out.Content = append(out.Content, k, v)
		}
	}
	for k := range pairs(base) {
		if !liveKeys[k.Value] {
			out.Content = append(out.Content, cloneNode(k), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"})
		}
	}
	return out
}

// nodeEqual compares values, not how they are written: 1.10 と "1.10" は同じとみなす｡
func nodeEqual(a, b *yaml.Node) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.ScalarNode:
		return a.Value == b.Value
	case yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !nodeEqual(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	case yaml.MappingNode:
		d := diff(a, b)
		return len(d.Content) == 0
	default:
		return false
	}
}

func removeKey(n *yaml.Node, key string) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return
		}
	}
}
