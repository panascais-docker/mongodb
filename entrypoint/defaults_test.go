package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestWithDefaults(t *testing.T) {
	for _, testCase := range []struct {
		arguments []string
		groups    [][]setting
		expected  []string
	}{
		{
			[]string{"mongod", "--timeStampFormat=iso8601-local", "--wiredTigerJournalCompressor", "snappy"},
			[][]setting{networkDefaults, storageDefaults, {{name: "directoryperdb"}}},
			[]string{
				"mongod", "--timeStampFormat=iso8601-local", "--wiredTigerJournalCompressor", "snappy",
				"--networkMessageCompressors", "zstd,snappy",
				"--wiredTigerCollectionBlockCompressor", "zstd",
				"--directoryperdb",
			},
		},
		{
			[]string{"mongod", "--setParameter", "enableTestCommands=1", "--setParameter=periodicNoopIntervalSecsExtra=1"},
			[][]setting{replicationDefaults},
			[]string{
				"mongod", "--setParameter", "enableTestCommands=1", "--setParameter=periodicNoopIntervalSecsExtra=1",
				"--setParameter", "periodicNoopIntervalSecs=1",
			},
		},
		{
			[]string{"mongod", "--setParameter", "periodicNoopIntervalSecs=5"},
			[][]setting{replicationDefaults},
			[]string{"mongod", "--setParameter", "periodicNoopIntervalSecs=5"},
		},
		{
			[]string{"mongod", "--setParameter=periodicNoopIntervalSecs=5"},
			[][]setting{replicationDefaults},
			[]string{"mongod", "--setParameter=periodicNoopIntervalSecs=5"},
		},
	} {
		if actual := withDefaults(slices.Clone(testCase.arguments), testCase.groups...); !slices.Equal(actual, testCase.expected) {
			t.Errorf("withDefaults(%q) = %q, expected %q", testCase.arguments, actual, testCase.expected)
		}
	}
}

func TestLayoutDefaults(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		options  bson.M
		expected []setting
	}{
		{"fresh", nil, []setting{{name: "directoryperdb"}, {name: "wiredTigerDirectoryForIndexes"}}},
		{"upstream layout", bson.M{"directoryPerDB": false, "directoryForIndexes": false}, nil},
		{"indexes only", bson.M{"directoryPerDB": false, "directoryForIndexes": true}, []setting{{name: "wiredTigerDirectoryForIndexes"}}},
	} {
		dbPath := t.TempDir()
		if testCase.options != nil {
			content, err := bson.Marshal(bson.M{"storage": bson.M{"engine": "wiredTiger", "options": testCase.options}})
			if err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(filepath.Join(dbPath, "storage.bson"), content, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		if actual := layoutDefaults(dbPath); !slices.Equal(actual, testCase.expected) {
			t.Errorf("%s: layoutDefaults() = %+v, expected %+v", testCase.name, actual, testCase.expected)
		}
	}
}
