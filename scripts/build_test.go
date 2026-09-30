package main

import (
	"slices"
	"testing"
)

func TestResolveNames(t *testing.T) {
	tags := configuration{
		"4.4": {"ubi8": "4.4.31-ubi8-slim"},
		"8.2": {"ubi8": "8.2.12-ubi8-slim", "ubi9": "8.2.12-ubi9-slim"},
		"8.3": {"ubi8": "8.3.11-ubi8-slim", "ubi9": "8.3.11-ubi9-slim", "ubi10": "8.3.11-ubi10-slim"},
		"9.0": {"ubi9": "9.0.2-ubi9-slim", "ubi10": "9.0.2-ubi10-slim"},
	}

	for _, testCase := range []struct {
		line, variant, version string
		expected               []string
	}{
		{"4.4", "ubi8", "4.4.31", []string{"4.4-ubi8", "4.4.31-ubi8", "4-ubi8", "4.4", "4.4.31", "4"}},
		{"8.2", "ubi9", "8.2.12", []string{"8.2-ubi9", "8.2.12-ubi9", "8.2", "8.2.12"}},
		{"8.3", "ubi8", "8.3.11", []string{"8.3-ubi8", "8.3.11-ubi8", "8-ubi8", "latest-ubi8"}},
		{"8.3", "ubi9", "8.3.11", []string{"8.3-ubi9", "8.3.11-ubi9", "8-ubi9", "8.3", "8.3.11", "8"}},
		{"9.0", "ubi10", "9.0.2", []string{"9.0-ubi10", "9.0.2-ubi10", "9-ubi10", "latest-ubi10"}},
		{"9.0", "ubi9", "9.0.2", []string{"9.0-ubi9", "9.0.2-ubi9", "9-ubi9", "latest-ubi9", "9.0", "9.0.2", "9", "latest"}},
	} {
		if actual := resolveNames(tags, testCase.line, testCase.variant, testCase.version); !slices.Equal(actual, testCase.expected) {
			t.Errorf("resolveNames(%s, %s) = %q, expected %q", testCase.line, testCase.variant, actual, testCase.expected)
		}
	}
}
