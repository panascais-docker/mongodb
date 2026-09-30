package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
)

const (
	digestsFile = "configuration/digests.json"
	tagsFile    = "configuration/tags.json"
	repository  = "mongodb/mongodb-community-server"
)

type configuration map[string]map[string]string

func (value configuration) set(line, variant, entry string) {
	if value[line] == nil {
		value[line] = map[string]string{}
	}

	value[line][variant] = entry
}

func (value configuration) equal(other configuration) bool {
	return maps.EqualFunc(value, other, maps.Equal)
}

func readConfiguration(path string) (configuration, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var value configuration
	if err := json.Unmarshal(content, &value); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return value, nil
}

func writeConfiguration(path string, value configuration) error {
	var compact bytes.Buffer
	compact.WriteByte('{')

	for lineIndex, line := range sortedKeys(value) {
		if lineIndex > 0 {
			compact.WriteByte(',')
		}

		compact.WriteString(strconv.Quote(line))
		compact.WriteString(":{")

		for variantIndex, variant := range sortedKeys(value[line]) {
			if variantIndex > 0 {
				compact.WriteByte(',')
			}

			compact.WriteString(strconv.Quote(variant))
			compact.WriteString(":")
			compact.WriteString(strconv.Quote(value[line][variant]))
		}

		compact.WriteByte('}')
	}

	compact.WriteByte('}')

	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "    "); err != nil {
		return err
	}

	indented.WriteByte('\n')

	return os.WriteFile(path, indented.Bytes(), 0o644)
}

func sortedKeys[Value any](record map[string]Value) []string {
	return slices.SortedFunc(maps.Keys(record), compareKeys)
}

func compareKeys(left, right string) int {
	return slices.Compare(keyNumbers(left), keyNumbers(right))
}

func keyNumbers(key string) []int {
	var numbers []int
	for part := range strings.SplitSeq(strings.TrimPrefix(key, "ubi"), ".") {
		number, _ := strconv.Atoi(part)
		numbers = append(numbers, number)
	}

	return numbers
}
