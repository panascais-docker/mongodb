package main

import (
	"slices"
	"testing"
)

func TestStaleLines(t *testing.T) {
	expected := configuration{
		"8.0": {"ghcr.io/x:8.0-ubi9": "sha256:a", "ghcr.io/x:8.0-ubi9-replica": "sha256:b"},
		"9.0": {"ghcr.io/x:9.0-ubi10": "sha256:c", "ghcr.io/x:9.0-ubi10-replica": "sha256:d"},
	}

	for _, testCase := range []struct {
		name      string
		published configuration
		force     bool
		lines     []string
	}{
		{"current", expected, false, []string{}},
		{"force", expected, true, []string{"8.0", "9.0"}},
		{"missing images", configuration{}, false, []string{"8.0", "9.0"}},
		{
			"missing flavor",
			configuration{"8.0": expected["8.0"], "9.0": {"ghcr.io/x:9.0-ubi10": "sha256:c"}},
			false,
			[]string{"9.0"},
		},
		{
			"stale flavor",
			configuration{"8.0": {"ghcr.io/x:8.0-ubi9": "sha256:a", "ghcr.io/x:8.0-ubi9-replica": "sha256:x"}, "9.0": expected["9.0"]},
			false,
			[]string{"8.0"},
		},
	} {
		if lines := staleLines(expected, testCase.published, testCase.force); lines == nil || !slices.Equal(lines, testCase.lines) {
			t.Errorf("staleLines(%s) = %v, expected %v", testCase.name, lines, testCase.lines)
		}
	}
}

func TestFingerprintWith(t *testing.T) {
	original := build{
		flavor:   "replica",
		image:    "mongodb/mongodb-community-server:9.0.2-ubi10-slim@sha256:a",
		builders: map[string]string{"GOLANG_IMAGE": "golang:1.27-alpine@sha256:b"},
		tags:     []string{"ghcr.io/x:9.0-ubi10-replica", "ghcr.io/x:latest-replica"},
	}
	expected := original.fingerprintWith("sha256:sources")

	for name, changed := range map[string]build{
		"image":    {flavor: original.flavor, image: "mongodb/mongodb-community-server:9.0.3-ubi10-slim@sha256:c", builders: original.builders, tags: original.tags},
		"builders": {flavor: original.flavor, image: original.image, builders: map[string]string{"GOLANG_IMAGE": "golang:1.27-alpine@sha256:d"}, tags: original.tags},
		"tags":     {flavor: original.flavor, image: original.image, builders: original.builders, tags: original.tags[:1]},
	} {
		if changed.fingerprintWith("sha256:sources") == expected {
			t.Errorf("changing the %s kept the fingerprint", name)
		}
	}

	if original.fingerprintWith("sha256:other") == expected {
		t.Error("changing the sources kept the fingerprint")
	}

	if original.fingerprintWith("sha256:sources") != expected {
		t.Error("the fingerprint is not deterministic")
	}
}
