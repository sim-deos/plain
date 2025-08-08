package cmd

import (
	"fmt"

	"github.com/sim-deos/plain/internal/app"

	"github.com/spf13/cobra"
)

func NewStartCmd(a *app.App) *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Starts a new feature",
		Long: `Starts a new faeture based off of the main branch by default to help starting a new feature quickly.
		To start a feature from a specific branch, use --from <branch-name>.
		All feature names must be one word, use hyphens where needed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runStart(a, cmd, args) },
	}
	c.Flags().StringP("from", "f", "main", "Base branch to start from")
	return c
}

func runStart(app *app.App, cmd *cobra.Command, args []string) error {
	feature := args[0]
	base, _ := cmd.Flags().GetString("from")

	if base == "here" {
		currentBranch, err := app.Git.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("cannot start feature from here: %w", err)
		}
		base = currentBranch
	}

	if err := app.Git.CreateBranch(feature, base); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	fmt.Printf("plain: started a new feature called %s based off of %s\n", feature, base)
	return nil
}
