package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	work, err := planned.actions()
	if err != nil {
		return err
	}
	// secret の値は書き込みを始める前に全部そろっているか確かめる (途中までの apply を避ける)
	values, err := secretValues(work)
	if err != nil {
		return err
	}
	if len(work) == 0 {
		return nil
	}
	writeRewrites(out, work)
	if !opts.yes && !confirm(cmd) {
		_, _ = fmt.Fprintln(out, "apply をやめた")
		return nil
	}
	x := executor{client: client, owner: planned.owner, secrets: values}
	for _, w := range work {
		for _, a := range w.actions {
			if err := x.execute(ctx, w.repo, a); err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintf(out, "applied %s\n", w.repo)
	}
	return nil
}

// repoActions is the writes to one repository.
type repoActions struct {
	repo    string
	actions []reconcile.Action
}

// actions turns every plan into writes, leaving out repositories with nothing to write.
func (p planned) actions() ([]repoActions, error) {
	var work []repoActions
	for _, e := range p.entries {
		acts, err := reconcile.Actions(e.plan, e.desired)
		if err != nil {
			return nil, err
		}
		if len(acts) > 0 {
			work = append(work, repoActions{repo: e.plan.Repo, actions: acts})
		}
	}
	return work, nil
}

// secretValues reads the environment variable of every secret to put, keyed by its name.
// 1 つでも無い (空も含む) ものがあれば､全部を挙げてエラーにする｡
func secretValues(work []repoActions) (map[string]string, error) {
	values := map[string]string{}
	var errs []error
	for _, w := range work {
		for _, a := range w.actions {
			if a.Op != reconcile.ActPutSecret {
				continue
			}
			env := a.Secret.FromEnv
			if v := os.Getenv(env); v != "" {
				values[env] = v
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %s %s の環境変数 %s が無い", w.repo, a.Kind, a.Name, env))
		}
	}
	return values, errors.Join(errs...)
}

// writeRewrites lists the secrets apply writes again. plan には出ないので､確認の前に見せる｡
func writeRewrites(out io.Writer, work []repoActions) {
	for _, w := range work {
		var names []string
		for _, a := range w.actions {
			if a.Op == reconcile.ActPutSecret {
				names = append(names, a.Kind+" "+a.Name)
			}
		}
		if len(names) > 0 {
			_, _ = fmt.Fprintf(out, "%s: %s を書き直す (値は比較できないので毎回)\n", w.repo, strings.Join(names, ", "))
		}
	}
}

// executor carries out actions. domain (reconcile) は GitHub を知らないので､ここで client に写す｡
type executor struct {
	client  *github.Client
	owner   string
	secrets map[string]string // 環境変数名 → 値
}

// secretStores maps the kinds of secrets to where they live.
var secretStores = map[string]github.SecretStore{
	reconcile.KindSecret:           github.SecretsActions,
	reconcile.KindDependabotSecret: github.SecretsDependabot,
}

func (x executor) execute(ctx context.Context, repo string, a reconcile.Action) error {
	c, owner := x.client, x.owner
	switch a.Op {
	case reconcile.ActCreateRepository:
		return c.CreateRepository(ctx, repo, a.Repository)
	case reconcile.ActUpdateRepository:
		return c.UpdateRepository(ctx, owner, repo, a.Fields)
	case reconcile.ActSetTopics:
		return c.SetTopics(ctx, owner, repo, a.Topics)
	case reconcile.ActUpsertRuleset:
		return c.UpsertRuleset(ctx, owner, repo, a.Name, a.Ruleset)
	case reconcile.ActDeleteRuleset:
		return c.DeleteRuleset(ctx, owner, repo, a.Name)
	case reconcile.ActCreateVariable:
		return c.CreateVariable(ctx, owner, repo, a.Name, a.Value)
	case reconcile.ActUpdateVariable:
		return c.UpdateVariable(ctx, owner, repo, a.Name, a.Value)
	case reconcile.ActDeleteVariable:
		return c.DeleteVariable(ctx, owner, repo, a.Name)
	case reconcile.ActPutSecret:
		return c.PutSecret(ctx, owner, repo, secretStores[a.Kind], a.Name, x.secrets[a.Secret.FromEnv])
	case reconcile.ActDeleteSecret:
		return c.DeleteSecret(ctx, owner, repo, secretStores[a.Kind], a.Name)
	default:
		return fmt.Errorf("unknown action %q", a.Op)
	}
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
