/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"os"

	"github.com/sim-deos/plain/cmd"
	"github.com/sim-deos/plain/internal/app"
	"github.com/sim-deos/plain/internal/git"
)

func main() {
	app := &app.App{Git: git.NewShellClient()}
	root := cmd.NewRootCmd(app)
	err := root.Execute()
	if err != nil {
		os.Exit(1)
	}
}
