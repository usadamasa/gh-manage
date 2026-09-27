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
	if err := cli.Execute(info.DisplayString()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
