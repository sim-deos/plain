/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/sim-deos/plain/internal/app"

	"github.com/spf13/cobra"
)

func NewDoneCmd(a *app.App) *cobra.Command {
	doneCmd := &cobra.Command{
		Use:   "done",
		Short: "A brief description of your command",
		Long:  `Not yet implemented`,
		Run: func(cmd *cobra.Command, args []string) {
			dirty, err := a.Git.IsBranchDirty()
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			if dirty {
				fmt.Println("branch is dirty")
			} else {
				fmt.Println("branch is clean")
			}
		},
	}
	return doneCmd
}
