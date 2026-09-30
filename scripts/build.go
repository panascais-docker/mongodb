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

type registry struct {
	host    string
	image   string
	secrets string
}

var (
	registries = []registry{
		{host: "ghcr.io", image: "ghcr.io/panascais-docker/mongodb/mongodb", secrets: "CONTAINER"},
		{image: "panascais/mongodb", secrets: "DOCKER"},
		{host: "quay.io", image: "quay.io/panascais/mongodb", secrets: "QUAY"},
	}
	flavors         = []struct{ name, suffix string }{{"standalone", ""}, {"replica", "-replica"}, {"cluster", "-cluster"}}
	defaultVariants = []string{"ubi10", "ubi9", "ubi8"}
	digestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type build struct {
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
		tag, digest := variants[variant], digests[line][variant]
		if err := validatePin(line, variant, tag, digest); err != nil {
			return nil, err
		}

		version, _, _ := strings.Cut(tag, "-")
		names := resolveNames(tags, line, variant, version)

		for _, flavor := range flavors {
			var imageTags []string
			for _, registry := range registries {
				for _, name := range names {
					imageTags = append(imageTags, registry.image+":"+name+flavor.suffix)
				}
			}

			builds = append(builds, build{
				flavor:  flavor.name,
				image:   repository + ":" + tag + "@" + digest,
				version: version,
				tags:    imageTags,
			})
		}
	}

	return builds, nil
}

func validatePin(line, variant, tag, digest string) error {
	tagPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(line) + `\.\d+-` + regexp.QuoteMeta(variant) + `-slim$`)
	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("invalid mongodb tag %q for %s %s", tag, line, variant)
	}

	if !digestPattern.MatchString(digest) {
		return fmt.Errorf("invalid mongodb digest %q for %s %s", digest, line, variant)
	}

	return nil
}

func resolveNames(tags configuration, line, variant, version string) []string {
	lines := sortedKeys(tags)
	withVariant := where(lines, func(candidate string) bool {
		_, found := tags[candidate][variant]

		return found
	})

	names := aliases(withVariant, line, version, "-"+variant)
	if defaultVariant(tags[line]) == variant {
		names = append(names, aliases(lines, line, version, "")...)
	}

	return names
}

func aliases(lines []string, line, version, suffix string) []string {
	major, _, _ := strings.Cut(line, ".")
	sameMajor := where(lines, func(candidate string) bool { return strings.HasPrefix(candidate, major+".") })

	names := []string{line + suffix, version + suffix}
	if slices.MaxFunc(sameMajor, compareKeys) == line {
		names = append(names, major+suffix)
	}

	if slices.MaxFunc(lines, compareKeys) == line {
		names = append(names, "latest"+suffix)
	}

	return names
}

func where(lines []string, keep func(string) bool) []string {
	return slices.DeleteFunc(slices.Clone(lines), func(line string) bool { return !keep(line) })
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

	if err := warmCache(builds, revision); err != nil {
		return err
	}

	for _, registry := range registries {
		if err := registry.login(); err != nil {
			return err
		}
	}

	for _, build := range builds {
		if err := run("docker", build.buildx(revision, "--push", "--platform", platforms)...); err != nil {
			return err
		}
	}

	return nil
}

func warmCache(builds []build, revision string) error {
	for _, build := range builds {
		if build.flavor != "cluster" {
			continue
		}

		if err := run("docker", build.buildx(revision, "--platform", platforms)...); err != nil {
			return err
		}
	}

	return nil
}

func buildLocally(builds []build) error {
	for _, build := range builds {
		if err := run("docker", build.buildx("local", "--load")...); err != nil {
			return err
		}

		if err := verifyImage(build.tags[0]); err != nil {
			return err
		}
	}

	return nil
}

func (build build) buildx(revision string, flags ...string) []string {
	arguments := []string{
		"buildx", "build",
		"--build-arg", "BUILD_DATE=" + time.Now().UTC().Format(time.RFC3339),
		"--build-arg", "MONGODB_IMAGE=" + build.image,
		"--build-arg", "MONGODB_VERSION=" + build.version,
		"--build-arg", "VCS_REF=" + revision,
		"--target", build.flavor,
		"--progress=plain",
	}

	for _, tag := range build.tags {
		arguments = append(arguments, "-t", tag)
	}

	return slices.Concat(arguments, flags, []string{"."})
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

func (registry registry) login() error {
	arguments := []string{"login"}
	if registry.host != "" {
		arguments = append(arguments, registry.host)
	}

	login := command("docker", append(arguments, "-u", os.Getenv(registry.secrets+"_REGISTRY_USERNAME"), "--password-stdin")...)
	login.Stdin = strings.NewReader(os.Getenv(registry.secrets + "_REGISTRY_TOKEN"))

	return login.Run()
}

func command(name string, arguments ...string) *exec.Cmd {
	process := exec.Command(name, arguments...)
	process.Stdout, process.Stderr = os.Stdout, os.Stderr

	return process
}

func run(name string, arguments ...string) error {
	return command(name, arguments...).Run()
}

func output(name string, arguments ...string) (string, error) {
	process := exec.Command(name, arguments...)
	process.Stderr = os.Stderr

	result, err := process.Output()

	return strings.TrimSpace(string(result)), err
}
