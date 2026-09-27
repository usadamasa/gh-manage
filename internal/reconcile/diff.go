package reconcile

import (
	"bytes"
	"strings"

	"go.yaml.in/yaml/v3"
)

// diffContext is the number of unchanged lines kept around each change.
const diffContext = 3

// diffGap marks unchanged lines left out between two hunks.
const diffGap = " ..."

// yamlLines renders v as YAML lines. nil は行を持たない (live に無いキーを足すとき)｡
func yamlLines(v any) []string {
	if v == nil {
		return nil
	}
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(v); err != nil {
		return []string{compact(v)}
	}
	return strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
}

// lineDiff returns the lines of a unified diff from a to b, each prefixed with ' ', '-' or '+'.
// 変更から diffContext 行より離れた行は省き､hunk の間だけ diffGap を挟む｡
func lineDiff(a, b []string) []string {
	ops := diffOps(a, b)
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op[0] == ' ' {
			continue
		}
		for j := max(0, i-diffContext); j <= min(len(ops)-1, i+diffContext); j++ {
			keep[j] = true
		}
	}
	var out []string
	last := -1
	for i, op := range ops {
		if !keep[i] {
			continue
		}
		if last >= 0 && i > last+1 {
			out = append(out, diffGap)
		}
		out = append(out, op)
		last = i
	}
	return out
}

// diffOps aligns a and b along their longest common subsequence. 変更の塊では削除を追加より先に並べる｡
func diffOps(a, b []string) []string {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []string
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			ops = append(ops, " "+a[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, "-"+a[i])
			i++
		default:
			ops = append(ops, "+"+b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, "-"+a[i])
	}
	for ; j < len(b); j++ {
		ops = append(ops, "+"+b[j])
	}
	return ops
}
