package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheSizeGB(t *testing.T) {
	for memory, expected := range map[uint64]string{
		0:                "",
		512 * mebibyte:   "0.25",
		1536 * mebibyte:  "0.25",
		2048 * mebibyte:  "0.5",
		2900 * mebibyte:  "0.91",
		3072 * mebibyte:  "1",
		4096 * mebibyte:  "1.5",
		5120 * mebibyte:  "2",
		65536 * mebibyte: "2",
	} {
		if actual := cacheSizeGB(memory); actual != expected {
			t.Errorf("cacheSizeGB(%d MiB) = %q, expected %q", memory/mebibyte, actual, expected)
		}
	}
}

func TestReadLimit(t *testing.T) {
	directory := t.TempDir()

	for content, expected := range map[string]uint64{
		"max\n":        0,
		"1073741824\n": 1024 * mebibyte,
	} {
		path := filepath.Join(directory, "memory.max")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		if actual := readLimit(path); actual != expected {
			t.Errorf("readLimit(%q) = %d, expected %d", content, actual, expected)
		}
	}

	if actual := readLimit(filepath.Join(directory, "missing")); actual != 0 {
		t.Errorf("readLimit(missing) = %d, expected 0", actual)
	}
}
