package cmd

import (
	"github.com/sim-deos/plain/internal/app"
	"github.com/spf13/cobra"
)

func NewRootCmd(a *app.App) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "plain",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
	examples and usage of using your application. For example:
	
	Cobra is a CLI library for Go that empowers applications.
	This application is a tool to generate the needed files
	to quickly create a Cobra application.`,
	}

	rootCmd.AddCommand(
		NewStartCmd(a),
		NewPreviewCmd(a),
		NewInitCmd(a),
		NewDoneCmd(a),
		NewCheckpointCmd(a),
	)
	return rootCmd
}
