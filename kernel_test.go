package main

import (
	"slices"
	"testing"
)

func TestRseqAffected(t *testing.T) {
	for release, expected := range map[string]bool{
		"6.18.9":                           false,
		"6.19.13-orbstack-00380-ga7e0a2dc": true,
		"7.0.13-generic":                   true,
		"7.0.14-orbstack-00380-ga7e0a2dc":  false,
		"7.1.0-rc3":                        false,
		"":                                 true,
		"garbage":                          true,
	} {
		if actual := rseqAffected(release); actual != expected {
			t.Errorf("rseqAffected(%q) = %v, expected %v", release, actual, expected)
		}
	}
}

func TestApplyRseqTunable(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		environment []string
		release     string
		expected    string
	}{
		{"unset, affected", nil, "7.0.1", "GLIBC_TUNABLES=glibc.pthread.rseq=1"},
		{"unset, fixed", nil, "7.0.14", "GLIBC_TUNABLES=glibc.pthread.rseq=0"},
		{"other tunables kept", []string{"GLIBC_TUNABLES=glibc.malloc.arena_max=2"}, "7.0.1", "GLIBC_TUNABLES=glibc.malloc.arena_max=2:glibc.pthread.rseq=1"},
		{"user value wins", []string{"GLIBC_TUNABLES=glibc.pthread.rseq=0"}, "7.0.1", "GLIBC_TUNABLES=glibc.pthread.rseq=0"},
	} {
		environment, _ := applyRseqTunable(append([]string{"HOME=/data/db"}, testCase.environment...), testCase.release)
		expected := []string{"HOME=/data/db", testCase.expected}

		if !slices.Equal(environment, expected) {
			t.Errorf("%s: applyRseqTunable() = %q, expected %q", testCase.name, environment, expected)
		}
	}
}
