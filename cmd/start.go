/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 1 {
			fmt.Println("plain: please provide a feature name")
			return
		}

		featureName := args[0]
		gitArgs := []string{"checkout", "-b", featureName}

		base, _ := cmd.Flags().GetString("from")

		if base == "here" {
			gitBranchCmd := exec.Command("git", "branch")
			if bs, err := gitBranchCmd.Output(); err != nil {
				fmt.Println("could not start feature from here")
				return
			} else {
				fmt.Println(string(bs))
			}
		}

		gitArgs = append(gitArgs, base)

		gitCmd := exec.Command("git", gitArgs...)
		gitCmd.Stdin = os.Stdin
		gitCmd.Stdout = os.Stdout
		gitCmd.Stderr = os.Stderr

		err := gitCmd.Run()
		if err != nil {
			fmt.Println("failed to start new faeture")
			return
		}
		fmt.Println("Starting a new feature called", featureName, ". Moved to a clean branch off of main")
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// startCmd.PersistentFlags().String("foo", "", "A help for foo")

	startCmd.Flags().StringP("from", "f", "main", "Base branch to start from")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// startCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
