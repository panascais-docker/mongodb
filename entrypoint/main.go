package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
)

const defaultPort = 27017

func main() {
	log.SetFlags(0)
	log.SetPrefix("entrypoint: ")

	command := standaloneCommand()
	command.AddCommand(healthcheckCommand(), replicaCommand(), clusterCommand())
	command.CompletionOptions.DisableDefaultCmd = true
	command.SilenceUsage = true

	if command.Execute() != nil {
		os.Exit(1)
	}
}

func portFlag(command *cobra.Command, port *int, usage string) {
	command.Flags().IntVar(port, "port", environmentInt("MONGODB_PORT", defaultPort), usage+", $MONGODB_PORT")
}

func environmentInt(name string, fallback int) int {
	value, found := os.LookupEnv(name)
	if !found {
		return fallback
	}

	number, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("%s must be a number, got %q", name, value)
	}

	return number
}

func execute(arguments, environment []string) {
	path, err := exec.LookPath(arguments[0])
	if err == nil {
		err = syscall.Exec(path, arguments, environment)
	}

	log.Fatal(err)
}
