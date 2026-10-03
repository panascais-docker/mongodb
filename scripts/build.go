package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

const (
	healthTimeout    = 60 * time.Second
	digestsDirectory = "digests"
)

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
	Args      map[string]string `json:"args"`
	Context   string            `json:"context"`
	Output    []string          `json:"output,omitzero"`
	Platforms []string          `json:"platforms"`
	Tags      []string          `json:"tags,omitzero"`
	Target    string            `json:"target"`
}

type export func(build build) (tags, outputs []string)

func buildCommand() *cobra.Command {
	var platform string

	command := &cobra.Command{
		Use:   "build <line>",
		Short: "Build every variant of a line, pushing by digest on GitHub Actions and smoke testing locally",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, arguments []string) error { return buildLine(arguments[0], platform) },
	}
	command.Flags().StringVar(&platform, "platform", "linux/"+runtime.GOARCH, "platform to build")

	return command
}

func buildLine(line, platform string) error {
	tags, digests, builders, err := readPins()
	if err != nil {
		return err
	}

	builds, err := planBuilds(line, tags, digests, builders)
	if err != nil {
		return err
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return push(line, builds, platform)
	}

	return buildLocally(builds, platform)
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

func (build build) target() string {
	return build.variant + "-" + build.flavor
}

func push(line string, builds []build, platform string) error {
	revision, err := output("git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}

	var images []string
	for _, registry := range registries {
		if err := registry.login(); err != nil {
			return err
		}

		images = append(images, registry.image)
	}

	metadataFile := filepath.Join(os.TempDir(), "mongodb-metadata.json")
	push := func(build) ([]string, []string) {
		return nil, []string{`type=image,"name=` + strings.Join(images, ",") + `",push-by-digest=true,name-canonical=true,push=true`}
	}
	if err := bake(builds, revision, platform, push, "--metadata-file", metadataFile); err != nil {
		return err
	}

	pushed, err := readPushedDigests(metadataFile, line, builds)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(digestsDirectory, 0o755); err != nil {
		return err
	}

	_, architecture, _ := strings.Cut(platform, "/")

	return writeConfiguration(filepath.Join(digestsDirectory, line+"-"+architecture+".json"), pushed)
}

func readPushedDigests(metadataFile, line string, builds []build) (configuration, error) {
	content, err := os.ReadFile(metadataFile)
	if err != nil {
		return nil, err
	}

	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(content, &metadata); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", metadataFile, err)
	}

	pushed := configuration{}
	for _, build := range builds {
		var result struct {
			Digest string `json:"containerimage.digest"`
		}
		if err := json.Unmarshal(metadata[build.target()], &result); err != nil || !digestPattern.MatchString(result.Digest) {
			return nil, fmt.Errorf("no pushed digest for %s in %s", build.target(), metadataFile)
		}

		pushed.set(line, build.target(), result.Digest)
	}

	return pushed, nil
}

func buildLocally(builds []build, platform string) error {
	load := func(build build) ([]string, []string) { return build.tags, nil }
	if err := bake(builds, "local", platform, load, "--load"); err != nil {
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

func bake(builds []build, revision, platform string, export export, flags ...string) error {
	definition, err := bakeDefinition(builds, revision, platform, export)
	if err != nil {
		return err
	}

	process := command("docker", slices.Concat([]string{"buildx", "bake", "--file", "-", "--progress=plain"}, flags)...)
	process.Stdin = bytes.NewReader(definition)

	return process.Run()
}

func bakeDefinition(builds []build, revision, platform string, export export) ([]byte, error) {
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

		tags, outputs := export(build)
		targets[build.target()] = bakeTarget{
			Args:      arguments,
			Context:   ".",
			Output:    outputs,
			Platforms: []string{platform},
			Tags:      tags,
			Target:    build.flavor,
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
	login.Stdout = os.Stderr
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
