package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/oklog/run"
)

type process struct {
	arguments []string
	after     <-chan struct{}
}

func serve(environment []string, processes []process, steps ...step) {
	var group run.Group
	group.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	for _, managed := range processes {
		if err := supervise(&group, managed, environment); err != nil {
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

func supervise(group *run.Group, managed process, environment []string) error {
	command := exec.Command(managed.arguments[0], managed.arguments[1:]...)
	command.Env = environment
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal := stopSignal(managed.arguments[0])

	if managed.after == nil {
		if err := command.Start(); err != nil {
			return err
		}

		group.Add(command.Wait, func(error) { _ = command.Process.Signal(signal) })

		return nil
	}

	var (
		mutex   sync.Mutex
		stopped bool
	)
	interrupted := make(chan struct{})

	group.Add(func() error {
		select {
		case <-managed.after:
		case <-interrupted:
			return nil
		}

		mutex.Lock()
		if stopped {
			mutex.Unlock()

			return nil
		}
		err := command.Start()
		mutex.Unlock()

		if err != nil {
			return err
		}

		return command.Wait()
	}, func(error) {
		mutex.Lock()
		defer mutex.Unlock()

		stopped = true
		close(interrupted)
		if command.Process != nil {
			_ = command.Process.Signal(signal)
		}
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
