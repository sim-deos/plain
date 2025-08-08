package cmd

import (
	"github.com/sim-deos/plain/internal/app"

	"github.com/spf13/cobra"
)

func NewInitCmd(a *app.App) *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initiates git tracking for this repository",
		Long:  `Not yet implemented`,
		RunE:  func(cmd *cobra.Command, args []string) error { return runInit(a, cmd, args) },
	}
	return initCmd
}

func runInit(a *app.App, cmd *cobra.Command, args []string) error {
	return a.Git.Init()
}
