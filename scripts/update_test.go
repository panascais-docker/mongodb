package main

import (
	"maps"
	"slices"
	"testing"
)

func TestChangedLines(t *testing.T) {
	digestsBefore := configuration{"8.0": {"ubi9": "sha256:a"}, "9.0": {"ubi9": "sha256:b"}, "4.4": {"ubi8": "sha256:c"}}
	tagsBefore := configuration{"8.0": {"ubi9": "8.0.1-ubi9-slim"}, "9.0": {"ubi9": "9.0.1-ubi9-slim"}, "4.4": {"ubi8": "4.4.1-ubi8-slim"}}

	digests := configuration{"8.0": {"ubi9": "sha256:a"}, "9.0": {"ubi9": "sha256:d"}, "9.1": {"ubi9": "sha256:e"}}
	tags := configuration{"8.0": {"ubi9": "8.0.1-ubi9-slim"}, "9.0": {"ubi9": "9.0.1-ubi9-slim"}, "9.1": {"ubi9": "9.1.0-ubi9-slim"}}

	if lines := changedLines(digests, tags, digestsBefore, tagsBefore); !slices.Equal(lines, []string{"9.0", "9.1"}) {
		t.Errorf("changedLines() = %v, expected [9.0 9.1]", lines)
	}

	if lines := changedLines(digests, tags, digests, tags); lines == nil || len(lines) != 0 {
		t.Errorf("changedLines() = %#v, expected an empty non-nil slice", lines)
	}
}

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
