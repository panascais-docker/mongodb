package main

import (
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
)

const (
	tunablesVariable = "GLIBC_TUNABLES="
	rseqTunable      = "glibc.pthread.rseq="
)

func mongodEnvironment() []string {
	release := kernelRelease()

	environment, affected := applyRseqTunable(os.Environ(), release)
	if affected {
		log.Printf("WARNING: kernel %q has the rseq regression (6.19 to 7.0.13), running tcmalloc without per-CPU caches", release)
	}

	return environment
}

func kernelRelease() string {
	release, _ := os.ReadFile("/proc/sys/kernel/osrelease")

	return strings.TrimSpace(string(release))
}

func rseqAffected(release string) bool {
	var major, minor, patch int
	if parsed, _ := fmt.Sscanf(release, "%d.%d.%d", &major, &minor, &patch); parsed < 2 {
		return true
	}

	return (major == 6 && minor >= 19) || (major == 7 && minor == 0 && patch < 14)
}

func applyRseqTunable(environment []string, release string) ([]string, bool) {
	affected := rseqAffected(release)

	tunable := rseqTunable + "0"
	if affected {
		tunable = rseqTunable + "1"
	}

	index := slices.IndexFunc(environment, func(entry string) bool { return strings.HasPrefix(entry, tunablesVariable) })
	if index < 0 {
		return append(environment, tunablesVariable+tunable), affected
	}

	tunables := strings.TrimPrefix(environment[index], tunablesVariable)
	if strings.Contains(tunables, rseqTunable) {
		return environment, affected
	}

	if tunables != "" {
		tunable = tunables + ":" + tunable
	}

	environment[index] = tunablesVariable + tunable

	return environment, affected
}
