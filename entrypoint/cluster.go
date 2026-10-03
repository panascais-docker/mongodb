package main

import (
	"log"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	configServerPort = 27018
	shardPort        = 27019
	configDBPath     = "/data/configdb"
	loopback         = "127.0.0.1"
)

func clusterCommand() *cobra.Command {
	var port int

	command := &cobra.Command{
		Use:    "cluster [flags] [-- mongod arguments...]",
		Short:  "Start mongos with a single node config server and shard, for CI only",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		Run:    func(_ *cobra.Command, arguments []string) { cluster(port, arguments) },
	}

	portFlag(command, &port, "mongos port")

	return command
}

func cluster(port int, arguments []string) {
	if _, err := exec.LookPath("mongos"); err != nil {
		log.Fatal("cluster needs mongos, which only the -cluster images ship")
	}

	if port == configServerPort || port == shardPort {
		log.Fatalf("port %d is taken by the config server or shard", port)
	}

	configServer := net.JoinHostPort(loopback, strconv.Itoa(configServerPort))
	shard := net.JoinHostPort(loopback, strconv.Itoa(shardPort))

	configMongod := slices.Concat([]string{"mongod", "--configsvr", "--replSet", "config", "--port", strconv.Itoa(configServerPort), "--dbpath", configDBPath, "--bind_ip", loopback}, processArguments("MONGODB_CONFIG_ARGUMENTS", arguments))
	shardMongod := slices.Concat([]string{"mongod", "--shardsvr", "--replSet", "shard", "--port", strconv.Itoa(shardPort), "--dbpath", defaultDBPath, "--bind_ip_all"}, processArguments("MONGODB_SHARD_ARGUMENTS", arguments))

	configReady := make(chan struct{})

	launch(port, []process{
		{arguments: withDefaults(configMongod, mongodDefaults(configDBPath), replicationDefaults, cacheDefaults(configMongod, configServerCacheSizeGB))},
		{arguments: withDefaults(shardMongod, mongodDefaults(defaultDBPath), replicationDefaults, cacheDefaults(shardMongod, cacheSizeGB(memoryLimit())))},
		{
			arguments: withDefaults(
				slices.Concat([]string{"mongos", "--configdb", "config/" + configServer, "--port", strconv.Itoa(port), "--bind_ip_all"}, processArguments("MONGODB_ROUTER_ARGUMENTS", nil)),
				networkDefaults,
			),
			after: configReady,
		},
	},
		initiate(configServerPort, "config", configServer, true),
		release(configReady),
		initiate(shardPort, "shard", shard, false),
		addShard(port, "shard/"+shard),
		createRoot(shardPort, loadRootCredential()),
	)
}

func processArguments(name string, shared []string) []string {
	return override(shared, strings.Fields(os.Getenv(name)))
}
