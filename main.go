/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"fmt"

	"github.com/sim-deos/plain/internal/git"
)

func main() {
	graph, err := git.GetHistoryFor("main")
	if err != nil {
		fmt.Println("git error: ", err.Error())
	}

	curr := graph.Head
	for {
		fmt.Printf("* %s: %s\n", curr.Hash[:7], curr.Message)
		if curr.IsEnd() {
			break
		}
		curr = graph.Commits[curr.Parents[0]]
	}
}
