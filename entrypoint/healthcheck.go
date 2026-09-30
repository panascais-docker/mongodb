package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func healthcheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "healthcheck",
		Short: "Ping mongod, logging in when a root user is configured",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return healthcheck() },
	}
}

func healthcheck() error {
	port, err := healthcheckPort()
	if err != nil {
		return err
	}

	client, err := dial(port, loadRootCredential())
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return client.Ping(ctx, nil)
}

func healthcheckPort() (int, error) {
	commandLine, _ := os.ReadFile("/proc/1/cmdline")
	arguments := strings.Split(string(commandLine), "\x00")

	if len(arguments) < 2 || !slices.Contains([]string{"replica", "cluster"}, arguments[1]) {
		return parseMongodFlags(arguments).port, nil
	}

	content, err := os.ReadFile(readyFile)
	if errors.Is(err, os.ErrNotExist) {
		return 0, errors.New("setup has not finished")
	}

	if err != nil {
		return 0, err
	}

	return strconv.Atoi(string(content))
}
