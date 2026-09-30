package main

import (
	"cmp"
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("entrypoint: ")

	command := &cobra.Command{
		Use:   "mongodb-entrypoint [command] [arguments...]",
		Short: "Start mongod, optionally with a root user",
		Long: `Start mongod, optionally with a root user.

Arguments that start with "-" are passed to mongod. Commands other than mongod
run as given.

Environment:
  MONGODB_ROOT_USERNAME, MONGODB_ROOT_PASSWORD  create a root user and enable --auth
                                                (each also as *_FILE)`,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		Run:                func(_ *cobra.Command, arguments []string) { standalone(arguments) },
	}
	command.AddCommand(&cobra.Command{
		Use:   "healthcheck",
		Short: "Ping mongod, logging in when a root user is configured",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return healthcheck() },
	})
	command.AddCommand(replicaCommand(), clusterCommand())
	command.CompletionOptions.DisableDefaultCmd = true
	command.SilenceUsage = true

	if command.Execute() != nil {
		os.Exit(1)
	}
}

func execute(arguments, environment []string) {
	path, err := exec.LookPath(arguments[0])
	if err == nil {
		err = syscall.Exec(path, arguments, environment)
	}

	log.Fatal(err)
}

func environmentString(name, fallback string) string {
	return cmp.Or(os.Getenv(name), fallback)
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
