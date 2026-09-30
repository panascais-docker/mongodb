package main

import (
	"maps"
	"testing"
)

func TestResolveReleases(t *testing.T) {
	releases := resolveReleases([]string{
		"4.2.24-ubi8-slim",
		"4.4.9-ubi8-slim",
		"4.4.31-ubi8-slim",
		"9.0.0-rc0-ubi9-slim",
		"9.0.2-ubi9-slim",
		"9.0.2-ubi9-slim-20260929T065623Z",
		"9.0.2-ubuntu2204-slim",
		"9.0.1-ubi10-slim",
		"9.0-ubi10-slim",
		"latest-slim",
	})

	expected := map[string]map[string]release{
		"4.4": {"ubi8": {patch: 31, tag: "4.4.31-ubi8-slim"}},
		"9.0": {"ubi9": {patch: 2, tag: "9.0.2-ubi9-slim"}, "ubi10": {patch: 1, tag: "9.0.1-ubi10-slim"}},
	}

	if !maps.EqualFunc(releases, expected, maps.Equal) {
		t.Errorf("resolveReleases() = %v, expected %v", releases, expected)
	}
}
