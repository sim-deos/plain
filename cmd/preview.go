package cmd

import (
	"fmt"

	"github.com/sim-deos/plain/internal/app"

	"github.com/spf13/cobra"
)

func NewPreviewCmd(a *app.App) *cobra.Command {
	previewCmd := &cobra.Command{
		Use:   "preview",
		Short: "A brief description of your command",
		Long:  `Long not yet implemented`,
		Run:   func(cmd *cobra.Command, args []string) { fmt.Println("preview called") },
	}
	return previewCmd
}
