package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/usadamasa/gh-manage/internal/config"
	"github.com/usadamasa/gh-manage/internal/github"
)

type snapshotOptions struct {
	repo            string
	minimize        bool
	includeArchived bool
	includeForks    bool
}

func newSnapshotCmd(settingsDir *string, newClient clientFactory) *cobra.Command {
	var opts snapshotOptions
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write the live settings to settings/repos/<name>.yaml",
		Long: `snapshot reads the live settings of your repositories and writes them as
overlays to settings/repos/<name>.yaml, overwriting existing files.
Secrets are written by name with a from_env placeholder, since their values cannot be read.
Without --repo it writes every repository you own, except archived ones and forks.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSnapshot(cmd, *settingsDir, newClient, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.repo, "repo", "", "snapshot only this repository")
	f.BoolVar(&opts.minimize, "minimize", false, "keep only what differs from base.yaml")
	f.BoolVar(&opts.includeArchived, "include-archived", false, "include archived repositories")
	f.BoolVar(&opts.includeForks, "include-forks", false, "include forks")
	return cmd
}

func runSnapshot(cmd *cobra.Command, settingsDir string, newClient clientFactory, opts snapshotOptions) error {
	encode := config.EncodeOverlay
	if opts.minimize {
		cfg, err := config.Load(settingsDir)
		if err != nil {
			return fmt.Errorf("--minimize は base.yaml と比べる: %w", err)
		}
		encode = cfg.Minimize
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	defer func() {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "rate limit remaining: %s\n", client.RateLimitRemaining())
	}()

	ctx := cmd.Context()
	owner, err := client.CurrentUser(ctx)
	if err != nil {
		return err
	}
	names, err := snapshotTargets(ctx, client, opts)
	if err != nil {
		return err
	}
	reposDir := filepath.Join(settingsDir, "repos")
	if err := os.MkdirAll(reposDir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", reposDir, err)
	}
	for _, name := range names {
		live, err := client.FetchSettings(ctx, owner, name)
		if err != nil {
			return err
		}
		data, err := encode(live)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		path := filepath.Join(reposDir, name+".yaml")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
	}
	return nil
}

func snapshotTargets(ctx context.Context, client *github.Client, opts snapshotOptions) ([]string, error) {
	if opts.repo != "" {
		return []string{opts.repo}, nil
	}
	repos, err := client.ListOwnedRepos(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, r := range repos {
		if (r.Archived && !opts.includeArchived) || (r.Fork && !opts.includeForks) {
			continue
		}
		names = append(names, r.Name)
	}
	return names, nil
}
