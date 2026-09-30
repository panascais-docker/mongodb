package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const defaultDBPath = "/data/db"

type setting struct {
	name  string
	value string
}

var (
	networkDefaults = []setting{
		{"networkMessageCompressors", "zstd,snappy"},
		{"timeStampFormat", "iso8601-utc"},
	}
	storageDefaults = []setting{
		{"wiredTigerJournalCompressor", "zstd"},
		{"wiredTigerCollectionBlockCompressor", "zstd"},
	}
	replicationDefaults = []setting{
		{"setParameter", "periodicNoopIntervalSecs=1"},
	}
	layoutFlags = []struct{ option, flag string }{
		{"directoryPerDB", "directoryperdb"},
		{"directoryForIndexes", "wiredTigerDirectoryForIndexes"},
	}
)

func mongodDefaults(dbPath string) []setting {
	return slices.Concat(networkDefaults, storageDefaults, layoutDefaults(dbPath))
}

func layoutDefaults(dbPath string) []setting {
	options, fresh := storageOptions(dbPath)

	var settings []setting
	for _, layout := range layoutFlags {
		if fresh || options[layout.option] == true {
			settings = append(settings, setting{name: layout.flag})
		}
	}

	return settings
}

func storageOptions(dbPath string) (bson.M, bool) {
	content, err := os.ReadFile(filepath.Join(dbPath, "storage.bson"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, true
	}

	if err != nil {
		log.Fatal(err)
	}

	var metadata struct {
		Storage struct {
			Options bson.M `bson:"options"`
		} `bson:"storage"`
	}

	if err := bson.Unmarshal(content, &metadata); err != nil {
		log.Fatalf("reading %s: %v", dbPath, err)
	}

	return metadata.Storage.Options, false
}

func withDefaults(arguments []string, groups ...[]setting) []string {
	for _, setting := range slices.Concat(groups...) {
		if passed(arguments, setting.name) {
			continue
		}

		arguments = append(arguments, "--"+setting.name)
		if setting.value != "" {
			arguments = append(arguments, setting.value)
		}
	}

	return arguments
}

func passed(arguments []string, name string) bool {
	return slices.ContainsFunc(arguments, func(argument string) bool {
		return argument == "--"+name || strings.HasPrefix(argument, "--"+name+"=")
	})
}
