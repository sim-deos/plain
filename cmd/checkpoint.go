/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/sim-deos/plain/internal/app"

	"github.com/spf13/cobra"
)

func NewCheckpointCmd(a *app.App) *cobra.Command {
	checkpointCmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Set up a checkpoint in your code history",
		Long:  `Todo`,
		Run:   func(cmd *cobra.Command, args []string) { fmt.Println("checkpoint called") },
	}
	return checkpointCmd
}
