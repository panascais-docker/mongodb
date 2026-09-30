package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"

	"github.com/oklog/run"
)

func serve(environment []string, processes [][]string, steps ...step) {
	var group run.Group
	group.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	for _, arguments := range processes {
		if err := supervise(&group, arguments, environment); err != nil {
			log.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	group.Add(func() error {
		for _, step := range steps {
			if err := step(ctx); err != nil {
				return fmt.Errorf("setting up: %w", err)
			}
		}

		<-ctx.Done()

		return nil
	}, func(error) { cancel() })

	os.Exit(exitCode(group.Run()))
}

func supervise(group *run.Group, arguments, environment []string) error {
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Env = environment
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr

	if err := command.Start(); err != nil {
		return err
	}

	group.Add(command.Wait, func(error) {
		_ = command.Process.Signal(stopSignal(arguments[0]))
	})

	return nil
}

func stopSignal(program string) syscall.Signal {
	if program == "mongos" {
		return syscall.SIGKILL
	}

	return syscall.SIGTERM
}

func exitCode(err error) int {
	if err == nil || errors.Is(err, run.ErrSignal) {
		return 0
	}

	exitError, exited := errors.AsType[*exec.ExitError](err)
	if !exited {
		log.Print(err)

		return 1
	}

	if status, known := exitError.Sys().(syscall.WaitStatus); known && status.Signaled() {
		return 128 + int(status.Signal())
	}

	return exitError.ExitCode()
}
