package main

import (
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	kibibyte        = 1 << 10
	mebibyte        = 1 << 20
	reservedMiB     = 1024
	minimumCacheMiB = 256
	maximumCacheMiB = 2048
)

func memoryLimit() uint64 {
	var smallest uint64
	for _, limit := range []uint64{
		readLimit("/sys/fs/cgroup/memory.max"),
		readLimit("/sys/fs/cgroup/memory/memory.limit_in_bytes"),
		physicalMemory(),
	} {
		if limit != 0 && (smallest == 0 || limit < smallest) {
			smallest = limit
		}
	}

	return smallest
}

func readLimit(path string) uint64 {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	limit, err := strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)
	if err != nil {
		return 0
	}

	return limit
}

func physicalMemory() uint64 {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	for line := range strings.Lines(string(content)) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}

		kibibytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}

		return kibibytes * kibibyte
	}

	return 0
}

func cacheSizeGB(memory uint64) string {
	if memory == 0 {
		return ""
	}

	sizeMiB := min(maximumCacheMiB, max(minimumCacheMiB, (int64(memory/mebibyte)-reservedMiB)/2))

	return strconv.FormatFloat(math.Floor(float64(sizeMiB)/1024*100)/100, 'f', -1, 64)
}
