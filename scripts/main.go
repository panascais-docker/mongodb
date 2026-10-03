package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	command := &cobra.Command{
		Use:          "scripts",
		Short:        "Build panascais/mongodb images and update their configuration",
		SilenceUsage: true,
	}
	command.AddCommand(buildCommand(), planCommand(), updateCommand())
	command.CompletionOptions.DisableDefaultCmd = true

	if command.Execute() != nil {
		os.Exit(1)
	}
}
