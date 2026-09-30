package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"syscall"

	"github.com/oklog/run"
)

func serve(processes [][]string, environment []string, setup func(context.Context) error) {
	var group run.Group
	group.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	for _, arguments := range processes {
		if err := supervise(&group, arguments, environment); err != nil {
			log.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	var setupErr error
	group.Add(func() error {
		if err := setup(ctx); err != nil {
			if ctx.Err() == nil {
				setupErr = err
			}

			return err
		}

		<-ctx.Done()

		return nil
	}, func(error) { cancel() })

	err := group.Run()
	if setupErr != nil {
		log.Fatalf("setting up: %v", setupErr)
	}

	os.Exit(exitCode(err))
}

func supervise(group *run.Group, arguments, environment []string) error {
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Env = environment
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr

	if err := command.Start(); err != nil {
		return err
	}

	// mongos is stateless and drains for 15s on SIGTERM, longer than docker stop waits
	stop := syscall.SIGTERM
	if arguments[0] == "mongos" {
		stop = syscall.SIGKILL
	}

	group.Add(command.Wait, func(error) {
		_ = command.Process.Signal(stop)
	})

	return nil
}

func exitCode(err error) int {
	if err == nil || errors.Is(err, run.ErrSignal) {
		return 0
	}

	exitError, exited := errors.AsType[*exec.ExitError](err)
	if !exited {
		return 1
	}

	if status, known := exitError.Sys().(syscall.WaitStatus); known && status.Signaled() {
		return 128 + int(status.Signal())
	}

	return exitError.ExitCode()
}
