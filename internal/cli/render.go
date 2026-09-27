package cli

import (
	"github.com/spf13/cobra"

	"github.com/usadamasa/gh-manage/internal/config"
)

func newRenderCmd(settingsDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "render [name]",
		Short: "Print the desired state (base merged with the overlay)",
		Long: `render merges settings/base.yaml with settings/repos/<name>.yaml and prints
the result as YAML. Without a name it prints every managed repository, keyed by name.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*settingsDir)
			if err != nil {
				return err
			}
			var v any
			if len(args) == 1 {
				v, err = cfg.Render(args[0])
			} else {
				v, err = cfg.RenderAll()
			}
			if err != nil {
				return err
			}
			out, err := config.Encode(v)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
}
