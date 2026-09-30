package main

import (
	"context"
	"log"
	"net"
	"os/exec"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
)

const (
	configServerPort = 27019
	shardPort        = 27018
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

	command.Flags().IntVar(&port, "port", environmentInt("MONGODB_PORT", 27017), "mongos port, $MONGODB_PORT")

	return command
}

func cluster(port int) {
	if _, err := exec.LookPath("mongos"); err != nil {
		log.Fatal("cluster needs mongos, which only the -cluster images ship")
	}

	if port == configServerPort || port == shardPort {
		log.Fatalf("port %d is taken by the config server or shard", port)
	}

	environment := processEnvironment()
	root := loadRootCredential()
	security := prepareSetup(root)

	configServer := net.JoinHostPort(loopback, strconv.Itoa(configServerPort))
	shard := net.JoinHostPort(loopback, strconv.Itoa(shardPort))

	processes := [][]string{
		slices.Concat([]string{"mongod", "--configsvr", "--replSet", "config", "--port", strconv.Itoa(configServerPort), "--dbpath", "/data/configdb", "--bind_ip", loopback}, security),
		slices.Concat([]string{"mongod", "--shardsvr", "--replSet", "shard", "--port", strconv.Itoa(shardPort), "--dbpath", "/data/db", "--bind_ip", loopback}, security),
		slices.Concat([]string{"mongos", "--configdb", "config/" + configServer, "--port", strconv.Itoa(port), "--bind_ip_all"}, security),
	}

	serve(processes, environment, func(ctx context.Context) error {
		if err := setupReplicaSet(ctx, configServerPort, "config", configServer, true); err != nil {
			return err
		}

		if err := setupReplicaSet(ctx, shardPort, "shard", shard, false); err != nil {
			return err
		}

		if err := setupShard(ctx, port, "shard/"+shard); err != nil {
			return err
		}

		if root != nil {
			if err := setupRoot(ctx, port, *root); err != nil {
				return err
			}
		}

		return markReady(port)
	})
}

func setupShard(ctx context.Context, port int, shard string) error {
	client, err := dial(port, nil)
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	if err := waitReady(ctx, client); err != nil {
		return err
	}

	return addShard(ctx, client, shard)
}
