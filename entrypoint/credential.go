package main

import (
	"log"
	"os"
	"strings"
)

type credential struct {
	username string
	password string
}

func loadRootCredential() *credential {
	username, password := secret("MONGODB_ROOT_USERNAME"), secret("MONGODB_ROOT_PASSWORD")
	if username == "" && password == "" {
		return nil
	}

	if username == "" || password == "" {
		log.Fatal("MONGODB_ROOT_USERNAME and MONGODB_ROOT_PASSWORD must be set together")
	}

	return &credential{username: username, password: password}
}

func secret(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	path := os.Getenv(name + "_FILE")
	if path == "" {
		return ""
	}

	value, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	return strings.TrimRight(string(value), "\r\n")
}
