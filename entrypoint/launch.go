package main

import (
	"crypto/rand"
	"errors"
	"log"
	"os"
)

const (
	keyFile   = "/tmp/mongodb-keyfile"
	readyFile = "/tmp/mongodb-ready"
)

func launch(port int, processes []process, steps ...step) {
	environment := mongodEnvironment()
	root := loadRootCredential()
	security := prepareRun(root)

	for index := range processes {
		processes[index].arguments = append(processes[index].arguments, security...)
	}

	serve(environment, processes, append(steps, createRoot(port, root), markReady(port))...)
}

func prepareRun(root *credential) []string {
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
