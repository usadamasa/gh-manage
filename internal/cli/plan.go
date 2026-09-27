package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/usadamasa/gh-manage/internal/config"
	"github.com/usadamasa/gh-manage/internal/github"
	"github.com/usadamasa/gh-manage/internal/reconcile"
)

// errDrift means plan found differences. main はこれを exit 2 にする｡
var errDrift = errors.New("differences found")

// ExitCode maps an error returned by Execute to the process exit code:
// 0 = 成功 (差分なし)､2 = 差分あり､1 = エラー｡
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errDrift):
		return 2
	default:
		return 1
	}
}

func newPlanCmd(settingsDir *string, newClient clientFactory) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show the difference between the declared and the live settings",
		Long: `plan reads the live settings of every managed repository and prints what apply would change.
Exit code: 0 = no difference, 2 = differences found, 1 = error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			write, ok := map[string]func(*cobra.Command, []reconcile.RepoPlan) error{
				"text": func(c *cobra.Command, p []reconcile.RepoPlan) error { return reconcile.WriteText(c.OutOrStdout(), p) },
				"json": func(c *cobra.Command, p []reconcile.RepoPlan) error { return reconcile.WriteJSON(c.OutOrStdout(), p) },
			}[format]
			if !ok {
				return fmt.Errorf("--format は text か json: %q", format)
			}
			cfg, err := config.Load(*settingsDir)
			if err != nil {
				return err
			}
			client, err := newClient()
			if err != nil {
				return err
			}
			plans, err := buildPlans(cmd.Context(), cfg, client)
			if err != nil {
				return err
			}
			if err := write(cmd, plans); err != nil {
				return err
			}
			for _, p := range plans {
				if p.HasChanges() {
					return errDrift
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}

// buildPlans plans every repository that has settings/repos/<name>.yaml.
func buildPlans(ctx context.Context, cfg *config.Config, client *github.Client) ([]reconcile.RepoPlan, error) {
	owner, err := client.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	repos, err := client.ListOwnedRepos(ctx)
	if err != nil {
		return nil, err
	}
	owned := make(map[string]github.Repo, len(repos))
	for _, r := range repos {
		owned[r.Name] = r
	}

	plans := make([]reconcile.RepoPlan, 0, len(cfg.Names()))
	for _, name := range cfg.Names() {
		desired, err := cfg.Render(name)
		if err != nil {
			return nil, err
		}
		repo, exists := owned[name]
		switch {
		case !exists:
			plans = append(plans, reconcile.Plan(name, desired, nil))
		case repo.Archived:
			plans = append(plans, reconcile.Skip(name, "archived なので skip"))
		default:
			live, err := client.FetchSettings(ctx, owner, name)
			if err != nil {
				return nil, err
			}
			plans = append(plans, reconcile.Plan(name, desired, live))
		}
	}
	return plans, nil
}
