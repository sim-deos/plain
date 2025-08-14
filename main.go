/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"fmt"
	"os"

	"github.com/sim-deos/plain/internal/git"
)

func main() {
	git := git.NewShellClient()
	branch, err := git.GetCurrentBranch()
	if err != nil {
		fmt.Println("could not get current branch")
	}
	fileBytes, err := os.ReadFile("./.git/refs/heads/" + branch)
	if err != nil {
		fmt.Println("ran into an error:", err.Error())
	}
	for idx, entry := range entries {
		fmt.Printf("Entry %d: %s\n", idx, entry.Name())
	}
}
