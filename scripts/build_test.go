package main

import (
	"maps"
	"slices"
	"strings"
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
		{"8.3", "ubi9", "8.3.11", []string{"8.3-ubi9", "8.3.11-ubi9", "8-ubi9"}},
		{"8.3", "ubi10", "8.3.11", []string{"8.3-ubi10", "8.3.11-ubi10", "8-ubi10", "8.3", "8.3.11", "8"}},
		{"9.0", "ubi10", "9.0.2", []string{"9.0-ubi10", "9.0.2-ubi10", "9-ubi10", "latest-ubi10", "9.0", "9.0.2", "9", "latest"}},
		{"9.0", "ubi9", "9.0.2", []string{"9.0-ubi9", "9.0.2-ubi9", "9-ubi9", "latest-ubi9"}},
	} {
		if actual := resolveNames(tags, testCase.line, testCase.variant, testCase.version); !slices.Equal(actual, testCase.expected) {
			t.Errorf("resolveNames(%s, %s) = %q, expected %q", testCase.line, testCase.variant, actual, testCase.expected)
		}
	}
}

func TestBuilderImages(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)

	for _, testCase := range []struct {
		builders configuration
		expected map[string]string
	}{
		{
			configuration{"alpine": {"3.24": digest}, "golang": {"1.27-alpine": digest}},
			map[string]string{"ALPINE_IMAGE": "alpine:3.24@" + digest, "GOLANG_IMAGE": "golang:1.27-alpine@" + digest},
		},
		{configuration{"alpine": {"3.24": ""}}, nil},
		{configuration{"alpine": {"3.23": digest, "3.24": digest}}, nil},
	} {
		if actual, err := builderImages(testCase.builders); !maps.Equal(actual, testCase.expected) || (err == nil) != (testCase.expected != nil) {
			t.Errorf("builderImages(%v) = %v, %v, expected %v", testCase.builders, actual, err, testCase.expected)
		}
	}
}

func TestPlanManifests(t *testing.T) {
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	tags := configuration{"9.0": {"ubi10": "9.0.2-ubi10-slim"}}
	digests := configuration{"9.0": {"ubi10": digest("a")}}
	builders := configuration{"alpine": {"3.24": digest("a")}}
	pushed := func(architectures ...string) map[string]configuration {
		value := map[string]configuration{}
		for index, architecture := range architectures {
			value[architecture] = configuration{}
			for _, flavor := range flavors {
				value[architecture].set("9.0", "ubi10-"+flavor.name, digest(string(rune('b'+index))))
			}
		}

		return value
	}

	creations, err := planManifests(pushed("amd64", "arm64"), tags, digests, builders, digest("e"))
	if err != nil {
		t.Fatal(err)
	}

	if len(creations) != len(flavors)*len(registries) {
		t.Fatalf("planManifests() = %d creations, expected %d", len(creations), len(flavors)*len(registries))
	}

	builds, err := planBuilds("9.0", tags, digests, builders)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"buildx", "imagetools", "create",
		"--annotation", "index:net.panascais.docker.mongodb.fingerprint=" + builds[1].fingerprintWith(digest("e")),
		"--tag", "panascais/mongodb:9.0-ubi10-replica",
		"--tag", "panascais/mongodb:9.0.2-ubi10-replica",
		"--tag", "panascais/mongodb:9-ubi10-replica",
		"--tag", "panascais/mongodb:latest-ubi10-replica",
		"--tag", "panascais/mongodb:9.0-replica",
		"--tag", "panascais/mongodb:9.0.2-replica",
		"--tag", "panascais/mongodb:9-replica",
		"--tag", "panascais/mongodb:latest-replica",
		"panascais/mongodb@" + digest("b"),
		"panascais/mongodb@" + digest("c"),
	}
	if !slices.Equal(creations[len(registries)+1], expected) {
		t.Errorf("planManifests() = %q, expected %q", creations[len(registries)+1], expected)
	}

	for _, testCase := range []map[string]configuration{pushed("amd64"), {}} {
		if _, err := planManifests(testCase, tags, digests, builders, digest("e")); err == nil {
			t.Errorf("planManifests(%v) succeeded, expected an error", testCase)
		}
	}
}
