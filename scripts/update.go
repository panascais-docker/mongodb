package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var (
	architectures  = []string{"amd64", "arm64"}
	minimumLine    = []int{4, 4}
	releasePattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)-(ubi\d+)-slim$`)
	client         = &http.Client{Timeout: 30 * time.Second}
)

type release struct {
	patch int
	tag   string
}

func updateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: `Pin the newest upstream image per line and variant, printing "continue" on changes and "exit" otherwise`,
		Args:  cobra.NoArgs,
		RunE:  func(command *cobra.Command, _ []string) error { return update(command.Context()) },
	}
}

func update(ctx context.Context) error {
	digestsBefore, err := readConfiguration(digestsFile)
	if err != nil {
		return err
	}

	tagsBefore, err := readConfiguration(tagsFile)
	if err != nil {
		return err
	}

	names, err := fetchTagNames(ctx)
	if err != nil {
		return err
	}

	digests, tags := configuration{}, configuration{}
	for line, variants := range resolveReleases(names) {
		for variant, release := range variants {
			digest, err := fetchDigest(ctx, release.tag)
			if err != nil {
				return err
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

	if digests.equal(digestsBefore) && tags.equal(tagsBefore) {
		fmt.Println("exit")

		return nil
	}

	if err := writeConfiguration(digestsFile, digests); err != nil {
		return err
	}

	if err := writeConfiguration(tagsFile, tags); err != nil {
		return err
	}

	fmt.Println("continue")

	return nil
}

func resolveReleases(names []string) map[string]map[string]release {
	releases := map[string]map[string]release{}
	for _, name := range names {
		match := releasePattern.FindStringSubmatch(name)
		if match == nil {
			continue
		}

		major, _ := strconv.Atoi(match[1])
		minor, _ := strconv.Atoi(match[2])
		patch, _ := strconv.Atoi(match[3])
		variant := match[4]

		if slices.Compare([]int{major, minor}, minimumLine) < 0 {
			continue
		}

		line := match[1] + "." + match[2]
		if releases[line] == nil {
			releases[line] = map[string]release{}
		}

		if current, found := releases[line][variant]; !found || current.patch < patch {
			releases[line][variant] = release{patch: patch, tag: name}
		}
	}

	return releases
}

func fetchTagNames(ctx context.Context) ([]string, error) {
	var token struct {
		Token string `json:"token"`
	}

	tokenURL := "https://auth.docker.io/token?service=registry.docker.io&scope=repository:" + repository + ":pull"
	if err := fetchJSON(ctx, tokenURL, "", &token); err != nil {
		return nil, err
	}

	var list struct {
		Tags []string `json:"tags"`
	}

	listURL := "https://registry-1.docker.io/v2/" + repository + "/tags/list"
	if err := fetchJSON(ctx, listURL, "Bearer "+token.Token, &list); err != nil {
		return nil, err
	}

	return list.Tags, nil
}

func fetchDigest(ctx context.Context, tag string) (string, error) {
	var details struct {
		Digest string `json:"digest"`
		Images []struct {
			Architecture string `json:"architecture"`
		} `json:"images"`
	}

	if err := fetchJSON(ctx, "https://hub.docker.com/v2/repositories/"+repository+"/tags/"+tag, "", &details); err != nil {
		return "", err
	}

	var available []string
	for _, image := range details.Images {
		available = append(available, image.Architecture)
	}

	for _, architecture := range architectures {
		if !slices.Contains(available, architecture) {
			return "", nil
		}
	}

	return details.Digest, nil
}

func fetchJSON(ctx context.Context, url, authorization string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: %s", url, response.Status)
	}

	return json.NewDecoder(response.Body).Decode(target)
}
