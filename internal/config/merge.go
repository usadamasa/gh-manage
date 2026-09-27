// Package config loads settings/base.yaml and settings/repos/<name>.yaml,
// merges them, and validates the result against the schema.
package config

import (
	"errors"

	"go.yaml.in/yaml/v3"
)

// merge deep-merges overlay into base and returns a new mapping node.
//
//   - map は再帰的に merge する
//   - overlay の null は base のキーを削除する
//   - list と scalar は overlay の値で丸ごと置き換える
//
// yaml.Node のまま merge するので scalar の書き方 (1.10 や "0755") は変わらない｡
// キーの順序は base の順で､overlay にだけあるキーは後ろに足す｡
// map の null は結果に残らない (list の要素の中の null は値として残す)｡
// base と overlay は変更しない｡nil は空の map として扱う｡
func merge(base, overlay *yaml.Node) *yaml.Node {
	var keys []*yaml.Node
	values := map[string]*yaml.Node{}
	put := func(k, v *yaml.Node) {
		if _, ok := values[k.Value]; !ok {
			keys = append(keys, cloneNode(k))
		}
		values[k.Value] = v
	}
	for k, v := range pairs(base) {
		if !isNull(v) {
			put(k, cloneNode(v))
		}
	}
	for k, v := range pairs(overlay) {
		old, ok := values[k.Value]
		switch {
		case isNull(v):
			delete(values, k.Value)
		case ok && old.Kind == yaml.MappingNode && v.Kind == yaml.MappingNode:
			values[k.Value] = merge(old, v)
		default:
			put(k, cloneNode(v))
		}
	}

	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range keys {
		if v, ok := values[k.Value]; ok {
			out.Content = append(out.Content, k, v)
		}
	}
	return out
}

// pairs iterates the key/value pairs of a mapping node. nil yields nothing.
func pairs(n *yaml.Node) func(yield func(k, v *yaml.Node) bool) {
	return func(yield func(k, v *yaml.Node) bool) {
		if n == nil {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if !yield(n.Content[i], n.Content[i+1]) {
				return
			}
		}
	}
}

// cloneNode deep-copies n, dropping null entries of maps and resolving aliases.
// list は丸ごと置き換えるので､要素の中の null は削除の指示でなく値として copyNode で残す｡
func cloneNode(n *yaml.Node) *yaml.Node {
	switch n.Kind {
	case yaml.MappingNode:
		return merge(n, nil)
	case yaml.AliasNode:
		return cloneNode(n.Alias)
	default:
		return copyNode(n)
	}
}

// copyNode deep-copies n as is, resolving aliases.
func copyNode(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode {
		return copyNode(n.Alias)
	}
	out := *n
	out.Anchor = ""
	if n.Content != nil {
		out.Content = make([]*yaml.Node, len(n.Content))
		for i, e := range n.Content {
			out.Content[i] = copyNode(e)
		}
	}
	return &out
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

// parseMapping parses a YAML document whose top level is a map.
// An empty document yields nil.
func parseMapping(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err //nolint:wrapcheck // 呼び出し側でファイル名を付ける
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	n := doc.Content[0]
	if n.Kind != yaml.MappingNode {
		return nil, errors.New("top level must be a map")
	}
	return n, nil
}
