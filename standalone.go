package main

import (
	"context"
	"log"
	"strings"

	"github.com/spf13/pflag"
)

type mongodFlags struct {
	auth   bool
	bindIP bool
	port   int
}

func standalone(arguments []string) {
	if len(arguments) == 0 || strings.HasPrefix(arguments[0], "-") {
		arguments = append([]string{"mongod"}, arguments...)
	}

	environment := processEnvironment()
	if arguments[0] != "mongod" {
		execute(arguments, environment)
	}

	flags := parseMongodFlags(arguments[1:])
	if !flags.bindIP {
		arguments = append(arguments, "--bind_ip_all")
	}

	root := loadRootCredential()
	if root == nil {
		execute(arguments, environment)
	}

	if !flags.auth {
		arguments = append(arguments, "--auth")
	}

	serve([][]string{arguments}, environment, func(ctx context.Context) error {
		return setupRoot(ctx, flags.port, *root)
	})
}

func setupRoot(ctx context.Context, port int, root credential) error {
	client, err := dial(port, nil)
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	if err := waitReady(ctx, client); err != nil {
		return err
	}

	created, err := createRoot(ctx, client, root)
	if created {
		log.Printf("created root user %q", root.username)
	}

	return err
}

func parseMongodFlags(arguments []string) mongodFlags {
	flagSet := pflag.NewFlagSet("mongod", pflag.ContinueOnError)
	flagSet.ParseErrorsAllowlist.UnknownFlags = true
	flagSet.Usage = func() {}

	auth := flagSet.Bool("auth", false, "")
	flagSet.String("bind_ip", "", "")
	flagSet.Bool("bind_ip_all", false, "")
	flagSet.StringP("config", "f", "", "")
	port := flagSet.Int("port", 27017, "")

	_ = flagSet.Parse(arguments)

	return mongodFlags{
		auth:   *auth,
		bindIP: flagSet.Changed("bind_ip") || flagSet.Changed("bind_ip_all") || flagSet.Changed("config"),
		port:   *port,
	}
}
