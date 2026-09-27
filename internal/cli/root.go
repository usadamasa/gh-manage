// Package cli defines the cobra commands of gh-manage.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/usadamasa/gh-manage/internal/github"
)

// clientFactory creates the GitHub client. テストは fake の RoundTripper を持つ client を渡す｡
type clientFactory func() (*github.Client, error)

func newRootCmd(displayVersion string) *cobra.Command {
	return newRootCmdWith(displayVersion, func() (*github.Client, error) {
		return github.New(github.Options{})
	})
}

func newRootCmdWith(displayVersion string, newClient clientFactory) *cobra.Command {
	root := &cobra.Command{
		Use:   "gh-manage",
		Short: "gh-manage - stateless configuration management for your GitHub repositories",
		Long: `gh-manage reconciles the settings of the repositories you own against
declarative YAML (settings/base.yaml + settings/repos/<name>.yaml).

It keeps no state: every run reads the live configuration from the GitHub API,
computes the difference, and applies only what changed.`,
		Version:       displayVersion,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	var settingsDir string
	root.PersistentFlags().StringVar(&settingsDir, "settings", "settings", "directory that holds base.yaml and repos/")
	root.AddCommand(newRenderCmd(&settingsDir))
	root.AddCommand(newSnapshotCmd(&settingsDir, newClient))
	root.AddCommand(newPlanCmd(&settingsDir, newClient))
	root.AddCommand(newApplyCmd(&settingsDir, newClient))
	return root
}

// Execute runs the root command with the given version string.
func Execute(displayVersion string) error {
	return newRootCmd(displayVersion).Execute()
}
