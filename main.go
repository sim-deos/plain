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
	// app := &app.App{Git: git.NewShellClient()}
	// root := cmd.NewRootCmd(app)
	// err := root.Execute()
	// if err != nil {
	// 	os.Exit(1)
	// }
	printGraph()
}

func printGraph() {
	history, err := git.GetHistoryFor("main")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	curr := history.Head
	for {
		fmt.Printf("* %s: %s\n", curr.Hash[0:7], curr.Message)
		if curr.IsEnd() {
			break
		}
		var next git.Commit
		if len(curr.Parents) > 1 {
			fmt.Printf("Commit %s has two parents: %s and %s\n", curr.DisName(), history.Graph[curr.Parents[0]].DisName(), history.Graph[curr.Parents[1]].DisName())
			next = history.Graph[curr.Parents[1]]
		} else {
			next = history.Graph[curr.Parents[0]]
		}
		curr = next
	}

}
