package main

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type mongodFlags struct {
	auth   bool
	bindIP bool
	config bool
	dbPath string
	port   int
}

func standaloneCommand() *cobra.Command {
	return &cobra.Command{
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
}

func standalone(arguments []string) {
	if len(arguments) == 0 || strings.HasPrefix(arguments[0], "-") {
		arguments = append([]string{"mongod"}, arguments...)
	}

	environment := mongodEnvironment()
	if arguments[0] != "mongod" {
		execute(arguments, environment)
	}

	flags := parseMongodFlags(arguments[1:])
	if !flags.bindIP {
		arguments = append(arguments, "--bind_ip_all")
	}

	if !flags.config {
		arguments = withDefaults(arguments, mongodDefaults(flags.dbPath))
	}

	root := loadRootCredential()
	if root == nil {
		execute(arguments, environment)
	}

	if !flags.auth {
		arguments = append(arguments, "--auth")
	}

	serve(environment, [][]string{arguments}, createRoot(flags.port, root))
}

func parseMongodFlags(arguments []string) mongodFlags {
	flagSet := pflag.NewFlagSet("mongod", pflag.ContinueOnError)
	flagSet.ParseErrorsAllowlist.UnknownFlags = true
	flagSet.Usage = func() {}

	auth := flagSet.Bool("auth", false, "")
	flagSet.String("bind_ip", "", "")
	flagSet.Bool("bind_ip_all", false, "")
	flagSet.StringP("config", "f", "", "")
	dbPath := flagSet.String("dbpath", defaultDBPath, "")
	port := flagSet.Int("port", defaultPort, "")

	_ = flagSet.Parse(arguments)

	config := flagSet.Changed("config")

	return mongodFlags{
		auth:   *auth,
		bindIP: config || flagSet.Changed("bind_ip") || flagSet.Changed("bind_ip_all"),
		config: config,
		dbPath: *dbPath,
		port:   *port,
	}
}
