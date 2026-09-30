package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"
)

const minimumLine = "4.4"

var (
	architectures  = []string{"amd64", "arm64"}
	releasePattern = regexp.MustCompile(`^(\d+\.\d+)\.(\d+)-(ubi\d+)-slim$`)
)

type release struct {
	patch int
	tag   string
}

func updateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: `Pin the newest upstream image per line and variant, printing the changed lines as a JSON array`,
		Args:  cobra.NoArgs,
		RunE:  func(command *cobra.Command, _ []string) error { return update(command.Context()) },
	}
}

func update(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	digestsBefore, err := readConfiguration(digestsFile)
	if err != nil {
		return err
	}

	tagsBefore, err := readConfiguration(tagsFile)
	if err != nil {
		return err
	}

	names, err := crane.ListTags(repository, dockerHub(ctx)...)
	if err != nil {
		return err
	}

	digests, tags, err := resolvePins(ctx, resolveReleases(names), digestsBefore, tagsBefore)
	if err != nil {
		return err
	}

	if digests.equal(digestsBefore) && tags.equal(tagsBefore) {
		fmt.Println("[]")

		return nil
	}

	if err := writeConfiguration(digestsFile, digests); err != nil {
		return err
	}

	if err := writeConfiguration(tagsFile, tags); err != nil {
		return err
	}

	lines, err := json.Marshal(changedLines(digests, tags, digestsBefore, tagsBefore))
	if err != nil {
		return err
	}

	fmt.Println(string(lines))

	return nil
}

func changedLines(digests, tags, digestsBefore, tagsBefore configuration) []string {
	lines := []string{}
	for _, line := range sortedKeys(tags) {
		if digests.changed(digestsBefore, line) || tags.changed(tagsBefore, line) {
			lines = append(lines, line)
		}
	}

	return lines
}

func resolvePins(ctx context.Context, releases map[string]map[string]release, digestsBefore, tagsBefore configuration) (configuration, configuration, error) {
	digests, tags := configuration{}, configuration{}
	for line, variants := range releases {
		for variant, release := range variants {
			digest, err := fetchDigest(ctx, release.tag, digestsBefore[line][variant])
			if err != nil {
				return nil, nil, err
			}

			tag := release.tag
			if digest == "" {
				digest, tag = digestsBefore[line][variant], tagsBefore[line][variant]
			}

			if digest == "" || tag == "" {
				continue
			}

			digests.set(line, variant, digest)
			tags.set(line, variant, tag)
		}
	}

	return digests, tags, nil
}

func resolveReleases(names []string) map[string]map[string]release {
	releases := map[string]map[string]release{}
	for _, name := range names {
		match := releasePattern.FindStringSubmatch(name)
		if match == nil || compareKeys(match[1], minimumLine) < 0 {
			continue
		}

		line, variant := match[1], match[3]
		patch, _ := strconv.Atoi(match[2])

		if releases[line] == nil {
			releases[line] = map[string]release{}
		}

		if current, found := releases[line][variant]; !found || current.patch < patch {
			releases[line][variant] = release{patch: patch, tag: name}
		}
	}

	return releases
}

func fetchDigest(ctx context.Context, tag, pinned string) (string, error) {
	head, err := crane.Head(repository+":"+tag, dockerHub(ctx)...)
	if err != nil || head.Digest.String() == pinned {
		return pinned, err
	}

	manifest, err := crane.Manifest(repository+"@"+head.Digest.String(), dockerHub(ctx)...)
	if err != nil {
		return "", err
	}

	index, err := v1.ParseIndexManifest(bytes.NewReader(manifest))
	if err != nil {
		return "", err
	}

	for _, architecture := range architectures {
		if !slices.ContainsFunc(index.Manifests, func(entry v1.Descriptor) bool {
			return entry.Platform != nil && entry.Platform.Architecture == architecture
		}) {
			return "", nil
		}
	}

	return head.Digest.String(), nil
}

func dockerHub(ctx context.Context) []crane.Option {
	return []crane.Option{crane.WithContext(ctx), func(options *crane.Options) {
		options.Name = append(options.Name, name.WithDefaultRegistry("registry-1.docker.io"))
		options.Remote = append(options.Remote, remote.WithPageSize(1_000_000))
	}}
}
