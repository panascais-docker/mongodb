package main

import (
	"cmp"
	"net"
	"os"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
)

type replicaOptions struct {
	name string
	host string
	port int
}

func replicaCommand() *cobra.Command {
	var options replicaOptions

	command := &cobra.Command{
		Use:    "replica [flags] [-- mongod arguments...]",
		Short:  "Start a single node replica set, for CI only",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		Run:    func(_ *cobra.Command, arguments []string) { replica(options, arguments) },
	}

	flags := command.Flags()
	flags.StringVar(&options.name, "name", cmp.Or(os.Getenv("MONGODB_REPLICA_SET"), "rs0"), "replica set name, $MONGODB_REPLICA_SET")
	flags.StringVar(&options.host, "host", cmp.Or(os.Getenv("MONGODB_REPLICA_HOST"), "localhost"), "host clients and mongod reach the member on, $MONGODB_REPLICA_HOST")
	portFlag(command, &options.port, "port")

	return command
}

func replica(options replicaOptions, arguments []string) {
	port := strconv.Itoa(options.port)
	mongod := slices.Concat([]string{"mongod", "--replSet", options.name, "--port", port, "--dbpath", defaultDBPath, "--bind_ip_all"}, arguments)
	mongod = withDefaults(mongod, mongodDefaults(defaultDBPath), replicationDefaults, cacheDefaults(mongod, cacheSizeGB(memoryLimit())))

	launch(options.port, []process{{arguments: mongod}},
		initiate(options.port, options.name, net.JoinHostPort(options.host, port), false),
	)
}
