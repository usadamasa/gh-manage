// Command gh-manage reconciles the settings of the repositories you own
// against declarative YAML.
package main

import (
	"fmt"
	"os"

	"github.com/usadamasa/gh-manage/internal/cli"
	"github.com/usadamasa/gh-manage/internal/version"
)

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

func main() {
	info := version.Resolve(buildVersion, buildCommit, buildDate)
	err := cli.Execute(info.DisplayString())
	code := cli.ExitCode(err)
	// exit 2 (plan の差分あり) は結果そのものなのでメッセージを出さない
	if code == 1 {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}
