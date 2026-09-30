package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
)

const (
	keyFile   = "/tmp/mongodb-keyfile"
	readyFile = "/tmp/mongodb-ready"
)

type replicaOptions struct {
	name string
	host string
	port int
}

func replicaCommand() *cobra.Command {
	var options replicaOptions

	command := &cobra.Command{
		Use:    "replica",
		Short:  "Start a single node replica set, for CI only",
		Hidden: true,
		Args:   cobra.NoArgs,
		Run:    func(*cobra.Command, []string) { replica(options) },
	}

	flags := command.Flags()
	flags.StringVar(&options.name, "name", environmentString("MONGODB_REPLICA_SET", "rs0"), "replica set name, $MONGODB_REPLICA_SET")
	flags.StringVar(&options.host, "host", environmentString("MONGODB_REPLICA_HOST", "localhost"), "host clients and mongod reach the member on, $MONGODB_REPLICA_HOST")
	flags.IntVar(&options.port, "port", environmentInt("MONGODB_PORT", 27017), "port, $MONGODB_PORT")

	return command
}

func replica(options replicaOptions) {
	environment := processEnvironment()
	root := loadRootCredential()
	security := prepareSetup(root)

	port := strconv.Itoa(options.port)
	mongod := slices.Concat([]string{"mongod", "--replSet", options.name, "--port", port, "--dbpath", "/data/db", "--bind_ip_all"}, security)

	serve([][]string{mongod}, environment, func(ctx context.Context) error {
		if err := setupReplicaSet(ctx, options.port, options.name, net.JoinHostPort(options.host, port), false); err != nil {
			return err
		}

		if root != nil {
			if err := setupRoot(ctx, options.port, *root); err != nil {
				return err
			}
		}

		return markReady(options.port)
	})
}

func prepareSetup(root *credential) []string {
	if err := os.Remove(readyFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	}

	if root == nil {
		return nil
	}

	if _, err := os.Stat(keyFile); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(keyFile, []byte(rand.Text()+rand.Text()), 0o400); err != nil {
			log.Fatal(err)
		}
	}

	return []string{"--keyFile", keyFile}
}

func setupReplicaSet(ctx context.Context, port int, name, host string, configServer bool) error {
	client, err := dial(port, nil)
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	if err := waitReady(ctx, client); err != nil {
		return err
	}

	if err := initiateReplicaSet(ctx, client, name, host, configServer); err != nil {
		return fmt.Errorf("initiating %s: %w", name, err)
	}

	return waitPrimary(ctx, client)
}

func markReady(port int) error {
	if err := os.WriteFile(readyFile, []byte(strconv.Itoa(port)), 0o644); err != nil {
		return err
	}

	log.Print("ready")

	return nil
}
