package main

import (
	"log"
	"net"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"
)

const (
	configServerPort = 27019
	shardPort        = 27018
	configDBPath     = "/data/configdb"
	loopback         = "127.0.0.1"
)

func clusterCommand() *cobra.Command {
	var port int

	command := &cobra.Command{
		Use:    "cluster",
		Short:  "Start mongos with a single node config server and shard, for CI only",
		Hidden: true,
		Args:   cobra.NoArgs,
		Run:    func(*cobra.Command, []string) { cluster(port) },
	}

	portFlag(command, &port, "mongos port")

	return command
}

func cluster(port int) {
	if _, err := exec.LookPath("mongos"); err != nil {
		log.Fatal("cluster needs mongos, which only the -cluster images ship")
	}

	if port == configServerPort || port == shardPort {
		log.Fatalf("port %d is taken by the config server or shard", port)
	}

	configServer := net.JoinHostPort(loopback, strconv.Itoa(configServerPort))
	shard := net.JoinHostPort(loopback, strconv.Itoa(shardPort))

	launch(port, [][]string{
		withDefaults(
			[]string{"mongod", "--configsvr", "--replSet", "config", "--port", strconv.Itoa(configServerPort), "--dbpath", configDBPath, "--bind_ip", loopback},
			mongodDefaults(configDBPath), replicationDefaults,
		),
		withDefaults(
			[]string{"mongod", "--shardsvr", "--replSet", "shard", "--port", strconv.Itoa(shardPort), "--dbpath", defaultDBPath, "--bind_ip", loopback},
			mongodDefaults(defaultDBPath), replicationDefaults,
		),
		withDefaults(
			[]string{"mongos", "--configdb", "config/" + configServer, "--port", strconv.Itoa(port), "--bind_ip_all"},
			networkDefaults,
		),
	},
		initiate(configServerPort, "config", configServer, true),
		initiate(shardPort, "shard", shard, false),
		addShard(port, "shard/"+shard),
	)
}
