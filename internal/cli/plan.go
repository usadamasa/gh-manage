package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

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
			planned, err := buildPlans(cmd.Context(), cfg, client)
			if err != nil {
				return err
			}
			plans := planned.plans()
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

// planned is the plans of every managed repository with what they were planned from.
type planned struct {
	owner   string
	entries []plannedRepo
}

type plannedRepo struct {
	plan    reconcile.RepoPlan
	desired *config.Settings
}

func (p planned) plans() []reconcile.RepoPlan {
	out := make([]reconcile.RepoPlan, len(p.entries))
	for i, e := range p.entries {
		out[i] = e.plan
	}
	return out
}

// buildPlans plans every repository that has settings/repos/<name>.yaml.
func buildPlans(ctx context.Context, cfg *config.Config, client *github.Client) (planned, error) {
	rendered, err := renderAll(cfg)
	if err != nil {
		return planned{}, err
	}
	owner, err := client.CurrentUser(ctx)
	if err != nil {
		return planned{}, err
	}
	repos, err := client.ListOwnedRepos(ctx)
	if err != nil {
		return planned{}, err
	}
	owned := make(map[string]github.Repo, len(repos))
	for _, r := range repos {
		owned[r.Name] = r
	}

	out := planned{owner: owner}
	for _, name := range cfg.Names() {
		desired := rendered[name]
		var p reconcile.RepoPlan
		repo, exists := owned[name]
		switch {
		case !exists:
			p = reconcile.Plan(name, desired, nil)
		case repo.Archived:
			p = reconcile.Skip(name, "archived なので skip")
		default:
			live, err := client.FetchSettings(ctx, owner, name)
			if err != nil {
				return planned{}, err
			}
			p = reconcile.Plan(name, desired, live)
		}
		out.entries = append(out.entries, plannedRepo{plan: p, desired: desired})
	}
	return out, nil
}

// renderAll renders every repository and reads the from_env variables from the environment.
// variable は plan で値を比べるので､secret と違い plan の時点で環境変数が要る｡
// 無いものがあれば､全部を挙げてエラーにする｡
func renderAll(cfg *config.Config) (map[string]*config.Settings, error) {
	out := map[string]*config.Settings{}
	var errs []error
	for _, name := range cfg.Names() {
		desired, err := cfg.Render(name)
		if err != nil {
			return nil, err
		}
		if err := desired.Variables.ResolveEnv(os.LookupEnv); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		out[name] = desired
	}
	return out, errors.Join(errs...)
}
