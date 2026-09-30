package main

import (
	"strings"
	"testing"
)

func TestParseMongodFlags(t *testing.T) {
	separator := func(character rune) bool { return character == ' ' || character == 0 }

	for arguments, expected := range map[string]mongodFlags{
		"":                                 {port: 27017},
		"--dbpath /data/x --port 5 --auth": {auth: true, port: 5},
		"--quiet --port=5":                 {port: 5},
		"-f /etc/mongod.conf":              {bindIP: true, port: 27017},
		"--bind_ip 127.0.0.1 --wiredTigerCacheSizeGB 1":       {bindIP: true, port: 27017},
		"mongodb-entrypoint\x00mongod\x00--port\x0027018\x00": {port: 27018},
	} {
		if actual := parseMongodFlags(strings.FieldsFunc(arguments, separator)); actual != expected {
			t.Errorf("parseMongodFlags(%q) = %+v, expected %+v", arguments, actual, expected)
		}
	}
}
