package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	healthTimeout = 60 * time.Second
	platforms     = "linux/amd64,linux/arm64"
)

var (
	images          = []string{"ghcr.io/panascais-docker/mongodb/mongodb", "panascais/mongodb", "quay.io/panascais/mongodb"}
	defaultVariants = []string{"ubi9", "ubi8"}
	flavors         = []string{"standalone", "replica", "cluster"}
	digestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type build struct {
	variant string
	flavor  string
	image   string
	version string
	tags    []string
}

func buildCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "build <line>",
		Short: "Build every variant of a line, pushing on GitHub Actions and smoke testing locally",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, arguments []string) error { return buildLine(arguments[0]) },
	}
}

func buildLine(line string) error {
	tags, err := readConfiguration(tagsFile)
	if err != nil {
		return err
	}

	digests, err := readConfiguration(digestsFile)
	if err != nil {
		return err
	}

	builds, err := planBuilds(line, tags, digests)
	if err != nil {
		return err
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return publish(builds)
	}

	return buildLocally(builds)
}

func planBuilds(line string, tags, digests configuration) ([]build, error) {
	variants, found := tags[line]
	if !found {
		return nil, fmt.Errorf("invalid line %q, expected one of %s", line, strings.Join(sortedKeys(tags), ", "))
	}

	var builds []build
	for _, variant := range sortedKeys(variants) {
		tag := variants[variant]
		tagPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(line) + `\.\d+-` + regexp.QuoteMeta(variant) + `-slim$`)
		if !tagPattern.MatchString(tag) {
			return nil, fmt.Errorf("invalid mongodb tag %q for %s %s", tag, line, variant)
		}

		digest := digests[line][variant]
		if !digestPattern.MatchString(digest) {
			return nil, fmt.Errorf("invalid mongodb digest %q for %s %s", digest, line, variant)
		}

		version, _, _ := strings.Cut(tag, "-")

		for _, flavor := range flavors {
			suffix := "-" + flavor
			if flavor == "standalone" {
				suffix = ""
			}

			var imageTags []string
			for _, image := range images {
				for _, name := range resolveNames(tags, line, variant, version) {
					imageTags = append(imageTags, image+":"+name+suffix)
				}
			}

			builds = append(builds, build{
				variant: variant,
				flavor:  flavor,
				image:   repository + ":" + tag + "@" + digest,
				version: version,
				tags:    imageTags,
			})
		}
	}

	return builds, nil
}

func resolveNames(tags configuration, line, variant, version string) []string {
	major, _, _ := strings.Cut(line, ".")

	suffixes := []string{"-" + variant}
	if defaultVariant(tags[line]) == variant {
		suffixes = append(suffixes, "")
	}

	var names []string
	for _, suffix := range suffixes {
		var candidates, majorCandidates []string
		for candidate, variants := range tags {
			if _, found := variants[variant]; !found && suffix != "" {
				continue
			}

			candidates = append(candidates, candidate)
			if strings.HasPrefix(candidate, major+".") {
				majorCandidates = append(majorCandidates, candidate)
			}
		}

		names = append(names, line+suffix, version+suffix)

		if slices.MaxFunc(majorCandidates, compareKeys) == line {
			names = append(names, major+suffix)
		}

		if slices.MaxFunc(candidates, compareKeys) == line {
			names = append(names, "latest"+suffix)
		}
	}

	return names
}

func defaultVariant(variants map[string]string) string {
	for _, variant := range defaultVariants {
		if _, found := variants[variant]; found {
			return variant
		}
	}

	return ""
}

func publish(builds []build) error {
	revision, err := output("git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}

	for _, build := range builds {
		if build.flavor != "cluster" {
			continue
		}

		if err := run("docker", buildArguments(build, revision, "--platform", platforms)...); err != nil {
			return err
		}
	}

	for _, registry := range []struct{ host, prefix string }{{"ghcr.io", "CONTAINER"}, {"", "DOCKER"}, {"quay.io", "QUAY"}} {
		if err := login(registry.host, os.Getenv(registry.prefix+"_REGISTRY_USERNAME"), os.Getenv(registry.prefix+"_REGISTRY_TOKEN")); err != nil {
			return err
		}
	}

	for _, build := range builds {
		if err := run("docker", buildArguments(build, revision, append([]string{"--push", "--platform", platforms}, tagArguments(build)...)...)...); err != nil {
			return err
		}
	}

	return nil
}

func buildLocally(builds []build) error {
	for _, build := range builds {
		if err := run("docker", buildArguments(build, "local", append([]string{"--load"}, tagArguments(build)...)...)...); err != nil {
			return err
		}

		if err := verifyImage(build.tags[0]); err != nil {
			return err
		}
	}

	return nil
}

func buildArguments(build build, revision string, extra ...string) []string {
	arguments := []string{
		"buildx", "build",
		"--build-arg", "BUILD_DATE=" + time.Now().UTC().Format(time.RFC3339),
		"--build-arg", "MONGODB_IMAGE=" + build.image,
		"--build-arg", "MONGODB_VERSION=" + build.version,
		"--build-arg", "VCS_REF=" + revision,
		"--target", build.flavor,
		"--progress=plain",
	}

	return append(append(arguments, extra...), ".")
}

func tagArguments(build build) []string {
	var arguments []string
	for _, tag := range build.tags {
		arguments = append(arguments, "-t", tag)
	}

	return arguments
}

func verifyImage(image string) error {
	for _, environment := range [][]string{nil, {"-e", "MONGODB_ROOT_USERNAME=root", "-e", "MONGODB_ROOT_PASSWORD=root"}} {
		if err := verifyContainer(image, environment); err != nil {
			return err
		}
	}

	return nil
}

func verifyContainer(image string, environment []string) error {
	container, err := output("docker", slices.Concat([]string{"run", "-d"}, environment, []string{image})...)
	if err != nil {
		return err
	}
	defer func() { _ = exec.Command("docker", "rm", "-f", container).Run() }()

	deadline := time.Now().Add(healthTimeout)
	for time.Now().Before(deadline) {
		health, err := output("docker", "inspect", "-f", "{{.State.Health.Status}}", container)
		if err != nil {
			return err
		}

		if health == "healthy" {
			return nil
		}

		time.Sleep(time.Second)
	}

	return fmt.Errorf("%s did not become healthy within %s", image, healthTimeout)
}

func login(registry, username, token string) error {
	arguments := []string{"login"}
	if registry != "" {
		arguments = append(arguments, registry)
	}

	command := exec.Command("docker", append(arguments, "-u", username, "--password-stdin")...)
	command.Stdin = strings.NewReader(token)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr

	return command.Run()
}

func run(name string, arguments ...string) error {
	command := exec.Command(name, arguments...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr

	return command.Run()
}

func output(name string, arguments ...string) (string, error) {
	command := exec.Command(name, arguments...)
	command.Stderr = os.Stderr

	result, err := command.Output()

	return strings.TrimSpace(string(result)), err
}
