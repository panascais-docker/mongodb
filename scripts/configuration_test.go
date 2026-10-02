package main

import (
	"slices"
	"testing"
)

func TestSortedKeys(t *testing.T) {
	for _, testCase := range []struct {
		record   map[string]string
		expected []string
	}{
		{map[string]string{"10.0": "", "9.0": "", "4.4": ""}, []string{"4.4", "9.0", "10.0"}},
		{map[string]string{"ubi10": "", "ubi9": "", "ubi8": ""}, []string{"ubi8", "ubi9", "ubi10"}},
		{map[string]string{"golang": "", "alpine": "", "busybox": ""}, []string{"alpine", "busybox", "golang"}},
	} {
		if actual := sortedKeys(testCase.record); !slices.Equal(actual, testCase.expected) {
			t.Errorf("sortedKeys(%v) = %q, expected %q", testCase.record, actual, testCase.expected)
		}
	}
}
