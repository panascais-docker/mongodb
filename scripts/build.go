package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

const healthTimeout = 60 * time.Second

type registry struct {
	host    string
	image   string
	secrets string
}

type flavor struct {
	name     string
	suffixes []string
}

var (
	registries = []registry{
		{host: "ghcr.io", image: "ghcr.io/panascais-docker/mongodb/mongodb", secrets: "CONTAINER"},
		{image: "panascais/mongodb", secrets: "DOCKER"},
		{host: "quay.io", image: "quay.io/panascais/mongodb", secrets: "QUAY"},
	}
	flavors = []flavor{
		{name: "standalone", suffixes: []string{"", "-standalone"}},
		{name: "replica", suffixes: []string{"-replica"}},
		{name: "cluster", suffixes: []string{"-cluster"}},
	}
	pushPlatforms   = []string{"linux/amd64", "linux/arm64"}
	defaultVariants = []string{"ubi10", "ubi9", "ubi8"}
	digestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type build struct {
	flavor      string
	image       string
	variant     string
	version     string
	builders    map[string]string
	tags        []string
	fingerprint string
}

type bakeFile struct {
	Group  map[string]bakeGroup  `json:"group"`
	Target map[string]bakeTarget `json:"target"`
}

type bakeGroup struct {
	Targets []string `json:"targets"`
}

type bakeTarget struct {
	Annotations []string          `json:"annotations,omitzero"`
	Args        map[string]string `json:"args"`
	Context     string            `json:"context"`
	Platforms   []string          `json:"platforms,omitzero"`
	Tags        []string          `json:"tags"`
	Target      string            `json:"target"`
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
	tags, digests, builders, err := readPins()
	if err != nil {
		return err
	}

	builds, err := planBuilds(line, tags, digests, builders)
	if err != nil {
		return err
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return publish(builds)
	}

	return buildLocally(builds)
}

func readPins() (configuration, configuration, configuration, error) {
	tags, err := readConfiguration(tagsFile)
	if err != nil {
		return nil, nil, nil, err
	}

	digests, err := readConfiguration(digestsFile)
	if err != nil {
		return nil, nil, nil, err
	}

	builders, err := readConfiguration(buildersFile)
	if err != nil {
		return nil, nil, nil, err
	}

	return tags, digests, builders, nil
}

func planBuilds(line string, tags, digests, builders configuration) ([]build, error) {
	variants, found := tags[line]
	if !found {
		return nil, fmt.Errorf("invalid line %q, expected one of %s", line, strings.Join(sortedKeys(tags), ", "))
	}

	images, err := builderImages(builders)
	if err != nil {
		return nil, err
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
				for _, suffix := range flavor.suffixes {
					for _, name := range names {
						imageTags = append(imageTags, registry.image+":"+name+suffix)
					}
				}
			}

			builds = append(builds, build{
				flavor:   flavor.name,
				image:    repository + ":" + tag + "@" + digest,
				variant:  variant,
				version:  version,
				builders: images,
				tags:     imageTags,
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

func builderImages(builders configuration) (map[string]string, error) {
	images := map[string]string{}
	for image, tags := range builders {
		if len(tags) != 1 {
			return nil, fmt.Errorf("invalid builder %s, expected exactly one tag", image)
		}

		for tag, digest := range tags {
			if !digestPattern.MatchString(digest) {
				return nil, fmt.Errorf("invalid builder digest %q for %s:%s", digest, image, tag)
			}

			images[strings.ToUpper(image)+"_IMAGE"] = image + ":" + tag + "@" + digest
		}
	}

	return images, nil
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

	for _, registry := range registries {
		if err := registry.login(); err != nil {
			return err
		}
	}

	fingerprinted, err := withFingerprints(builds)
	if err != nil {
		return err
	}

	return bake(fingerprinted, revision, pushPlatforms, "--push")
}

func buildLocally(builds []build) error {
	if err := bake(builds, "local", nil, "--load"); err != nil {
		return err
	}

	errs := make([]error, len(builds))

	var group sync.WaitGroup
	for index, build := range builds {
		group.Go(func() { errs[index] = verifyImage(build.tags[0]) })
	}
	group.Wait()

	return errors.Join(errs...)
}

func bake(builds []build, revision string, platforms []string, mode string) error {
	definition, err := bakeDefinition(builds, revision, platforms)
	if err != nil {
		return err
	}

	process := command("docker", "buildx", "bake", "--file", "-", "--progress=plain", mode)
	process.Stdin = bytes.NewReader(definition)

	return process.Run()
}

func bakeDefinition(builds []build, revision string, platforms []string) ([]byte, error) {
	date := time.Now().UTC().Format(time.RFC3339)

	targets := make(map[string]bakeTarget, len(builds))
	for _, build := range builds {
		arguments := map[string]string{
			"BUILD_DATE":      date,
			"MONGODB_IMAGE":   build.image,
			"MONGODB_VERSION": build.version,
			"VCS_REF":         revision,
		}
		maps.Copy(arguments, build.builders)

		var annotations []string
		if build.fingerprint != "" {
			annotations = []string{"index:" + fingerprintAnnotation + "=" + build.fingerprint}
		}

		targets[build.variant+"-"+build.flavor] = bakeTarget{
			Annotations: annotations,
			Args:        arguments,
			Context:     ".",
			Platforms:   platforms,
			Tags:        build.tags,
			Target:      build.flavor,
		}
	}

	return json.Marshal(bakeFile{
		Group:  map[string]bakeGroup{"default": {Targets: slices.Sorted(maps.Keys(targets))}},
		Target: targets,
	})
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

func output(name string, arguments ...string) (string, error) {
	process := exec.Command(name, arguments...)
	process.Stderr = os.Stderr

	result, err := process.Output()

	return strings.TrimSpace(string(result)), err
}
