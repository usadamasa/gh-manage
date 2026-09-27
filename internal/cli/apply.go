package cli

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/usadamasa/gh-manage/internal/config"
	"github.com/usadamasa/gh-manage/internal/github"
	"github.com/usadamasa/gh-manage/internal/reconcile"
)

type applyOptions struct {
	yes          bool
	allowPublish bool
}

func newApplyCmd(settingsDir *string, newClient clientFactory) *cobra.Command {
	var opts applyOptions
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply the plan to GitHub",
		Long: `apply computes the same plan as plan, shows it, asks for confirmation and then
writes the changes. Archived repositories are skipped.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runApply(cmd, *settingsDir, newClient, opts)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&opts.yes, "yes", false, "apply without asking for confirmation (for CI)")
	f.BoolVar(&opts.allowPublish, "allow-publish", false, "allow changing a private repository to public")
	return cmd
}

func runApply(cmd *cobra.Command, settingsDir string, newClient clientFactory, opts applyOptions) error {
	cfg, err := config.Load(settingsDir)
	if err != nil {
		return err
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	planned, err := buildPlans(ctx, cfg, client)
	if err != nil {
		return err
	}
	plans := planned.plans()
	if !opts.allowPublish {
		// 1 件でも該当すれば､何も書き込まずに止める
		if err := reconcile.CheckPublish(plans); err != nil {
			return err
		}
	}
	out := cmd.OutOrStdout()
	if err := reconcile.WriteText(out, plans); err != nil {
		return err
	}
	if !anyChanges(plans) {
		return nil
	}
	if !opts.yes && !confirm(cmd) {
		_, _ = fmt.Fprintln(out, "apply をやめた")
		return nil
	}
	for _, e := range planned.entries {
		if !e.plan.HasChanges() {
			continue
		}
		acts, err := reconcile.Actions(e.plan, e.desired)
		if err != nil {
			return err
		}
		for _, a := range acts {
			if err := execute(ctx, client, planned.owner, e.plan.Repo, a); err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintf(out, "applied %s\n", e.plan.Repo)
	}
	return nil
}

// execute carries out one action. domain (reconcile) は GitHub を知らないので､ここで client に写す｡
func execute(ctx context.Context, client *github.Client, owner, repo string, a reconcile.Action) error {
	switch a.Op {
	case reconcile.ActCreateRepository:
		return client.CreateRepository(ctx, repo, a.Repository)
	case reconcile.ActUpdateRepository:
		return client.UpdateRepository(ctx, owner, repo, a.Fields)
	case reconcile.ActSetTopics:
		return client.SetTopics(ctx, owner, repo, a.Topics)
	case reconcile.ActUpsertRuleset:
		return client.UpsertRuleset(ctx, owner, repo, a.Name, a.Ruleset)
	case reconcile.ActDeleteRuleset:
		return client.DeleteRuleset(ctx, owner, repo, a.Name)
	default:
		return fmt.Errorf("unknown action %q", a.Op)
	}
}

func anyChanges(plans []reconcile.RepoPlan) bool {
	for _, p := range plans {
		if p.HasChanges() {
			return true
		}
	}
	return false
}

// confirm asks on stdout and reads the answer from stdin. y / yes だけを了承とみなす｡
func confirm(cmd *cobra.Command) bool {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "apply しますか? [y/N] ")
	answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
