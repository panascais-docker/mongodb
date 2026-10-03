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

const (
	defaultDBPath           = "/data/db"
	configServerCacheSizeGB = "0.25"
)

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
	parameterDefaults = []setting{
		{"setParameter", "periodicNoopIntervalSecs=1"},
	}
	replicationDefaults = []setting{
		{"setParameter", "enableTestCommands=1"},
		{"oplogSize", "990"},
	}
	layoutFlags = []struct{ option, flag string }{
		{"directoryPerDB", "directoryperdb"},
		{"directoryForIndexes", "wiredTigerDirectoryForIndexes"},
	}
)

func mongodDefaults(dbPath string) []setting {
	return slices.Concat(networkDefaults, storageDefaults, parameterDefaults, layoutDefaults(dbPath))
}

func cacheDefaults(arguments []string, sizeGB string) []setting {
	if sizeGB == "" || passed(arguments, setting{name: "wiredTigerCacheSizePct"}) {
		return nil
	}

	return []setting{{"wiredTigerCacheSizeGB", sizeGB}}
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
		if passed(arguments, setting) {
			continue
		}

		arguments = append(arguments, "--"+setting.name)
		if setting.value != "" {
			arguments = append(arguments, setting.value)
		}
	}

	return arguments
}

func passed(arguments []string, setting setting) bool {
	flag := "--" + setting.name
	for index, argument := range arguments {
		value, joined := strings.CutPrefix(argument, flag+"=")
		if !joined && argument != flag {
			continue
		}

		if setting.name != "setParameter" {
			return true
		}

		if !joined && index+1 < len(arguments) {
			value = arguments[index+1]
		}

		if parameterName(value) == parameterName(setting.value) {
			return true
		}
	}

	return false
}

func override(arguments, overrides []string) []string {
	var kept []string
	for index := 0; index < len(arguments); index++ {
		group := arguments[index : index+1]
		name, value, joined := strings.Cut(arguments[index], "=")
		if !joined && index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "-") {
			group, value = arguments[index:index+2], arguments[index+1]
			index++
		}

		flag, isFlag := strings.CutPrefix(name, "--")
		if !isFlag || !passed(overrides, setting{flag, value}) {
			kept = append(kept, group...)
		}
	}

	return slices.Concat(kept, overrides)
}

func parameterName(assignment string) string {
	name, _, _ := strings.Cut(assignment, "=")

	return name
}
