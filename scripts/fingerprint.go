package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const fingerprintAnnotation = "net.panascais.docker.mongodb.fingerprint"

func sourcesFingerprint() (string, error) {
	entrypoint, err := filepath.Glob("entrypoint/*.go")
	if err != nil {
		return "", err
	}

	paths := slices.Concat([]string{"Dockerfile", "go.mod", "go.sum"}, slices.DeleteFunc(entrypoint, func(path string) bool {
		return strings.HasSuffix(path, "_test.go")
	}))

	values := map[string]string{}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}

		sum := sha256.Sum256(content)
		values[filepath.ToSlash(path)] = hex.EncodeToString(sum[:])
	}

	return fingerprint(values), nil
}

func withFingerprints(builds []build) ([]build, error) {
	sources, err := sourcesFingerprint()
	if err != nil {
		return nil, err
	}

	fingerprinted := slices.Clone(builds)
	for index := range fingerprinted {
		fingerprinted[index].fingerprint = fingerprinted[index].fingerprintWith(sources)
	}

	return fingerprinted, nil
}

func (build build) fingerprintWith(sources string) string {
	values := map[string]string{
		"flavor":  build.flavor,
		"image":   build.image,
		"sources": sources,
		"tags":    strings.Join(build.tags, " "),
	}
	maps.Copy(values, build.builders)

	return fingerprint(values)
}

func fingerprint(values map[string]string) string {
	hash := sha256.New()
	for _, key := range slices.Sorted(maps.Keys(values)) {
		_, _ = fmt.Fprintf(hash, "%s=%s\n", key, values[key])
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
