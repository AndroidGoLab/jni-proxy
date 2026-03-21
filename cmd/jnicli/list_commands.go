package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var listCommandsCmd = &cobra.Command{
	Use:    "list-commands",
	Short:  "List all leaf commands as full paths",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		printLeafCommands(rootCmd, nil)
	},
}

func printLeafCommands(cmd *cobra.Command, path []string) {
	current := make([]string, len(path)+1)
	copy(current, path)
	current[len(path)] = cmd.Use

	if !cmd.HasSubCommands() && cmd.RunE != nil {
		fmt.Println(strings.Join(current, " "))
		return
	}

	for _, child := range cmd.Commands() {
		if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		printLeafCommands(child, current)
	}
}

func init() {
	rootCmd.AddCommand(listCommandsCmd)
}
