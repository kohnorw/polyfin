package main

import (
	"bufio"
	"log/slog"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/cache"
)

// The memory Polyfin takes in a container with a memory limit, and
// GOMEMLIMIT unset:
//
//   - goShare: Go's soft limit is half the container's. FFmpeg, and the
//     PostgreSQL Polyfin runs with POLYFIN_DATABASE=embedded, count in the
//     same limit: one conversion takes 200 to 400 MB, a 4K one with HDR
//     tone mapped on the processor about 1 GB.
//   - fullCachesAt: the memory caches' budgets add up to about 1.2 GiB:
//     pages, descriptions, searches, stream and subtitle lists, smart
//     lists and artwork, 0.65 GiB; TMDB answers, the tracks, fonts and
//     indexes of files being played, and the rest, 0.5 GiB. They keep
//     them whole from a soft limit of 4 GiB, a container of 8 GiB. Under
//     it, they keep the share of them the soft limit is of 4 GiB: 30 % of
//     the soft limit at most, the rest of it left to the work under way,
//     such as decoding addons' answers and resizing artwork.
//   - leastCacheShare: they keep a thirty-second of their budgets at
//     least, 37 MiB, in a container of 256 MiB or less.
const (
	goShare         = 0.5
	fullCachesAt    = 4 << 30
	leastCacheShare = 1.0 / 32
)

// memoryPlan is the memory Polyfin takes: Go's soft limit, 0 for none,
// whether Polyfin sets it, and the share of their budgets its memory
// caches keep (see cache.SetBudgetShare).
type memoryPlan struct {
	limit int64
	set   bool
	share float64
}

// planMemory is the memory plan of a Polyfin whose Go runtime has the
// soft limit current, math.MaxInt64 for none, which GOMEMLIMIT set when
// fromEnv, in a container limited to container bytes, 0 for none: Go's
// soft limit is goShare of the container's unless GOMEMLIMIT sets one,
// and the caches' share follows the soft limit.
func planMemory(fromEnv bool, current, container int64) memoryPlan {
	var plan memoryPlan
	switch {
	case fromEnv && current < math.MaxInt64:
		plan.limit = current
	case !fromEnv && container > 0:
		plan.limit, plan.set = int64(float64(container)*goShare), true
	}
	plan.share = 1
	if plan.limit > 0 {
		plan.share = min(max(float64(plan.limit)/fullCachesAt, leastCacheShare), 1)
	}
	return plan
}

// limitMemory sets Go's soft memory limit from the container's on Linux,
// when GOMEMLIMIT sets none, and the share of their budgets the memory
// caches keep, and logs them. It runs before Polyfin makes its caches.
func limitMemory(logger *slog.Logger) {
	_, fromEnv := os.LookupEnv("GOMEMLIMIT")
	var container int64
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/self/cgroup")
		if err == nil {
			container = cgroupMemoryLimit("/sys/fs/cgroup", string(data))
		}
	}
	plan := planMemory(fromEnv, debug.SetMemoryLimit(-1), container)
	if plan.set {
		debug.SetMemoryLimit(plan.limit)
	}
	cache.SetBudgetShare(plan.share)
	share := strconv.FormatFloat(plan.share, 'g', 3, 64)
	switch {
	case plan.set:
		logger.Info("Go's memory limit is half the container's", "limit_mib", plan.limit>>20, "container_mib", container>>20, "cache_share", share)
	case plan.limit > 0:
		logger.Info("Go's memory limit is GOMEMLIMIT's", "limit_mib", plan.limit>>20, "cache_share", share)
	}
}

// cgroupMemoryLimit is the memory limit of the cgroup cgroups, the
// content of /proc/self/cgroup, names, under root, where the cgroup file
// systems are: the lowest of the cgroup's and its parents', from
// memory.max with cgroup v2, else from memory.limit_in_bytes with v1. 0
// when there is none. In a container, the cgroup is the container's.
func cgroupMemoryLimit(root, cgroups string) int64 {
	var v2, v1 string
	found1 := false
	scanner := bufio.NewScanner(strings.NewReader(cgroups))
	for scanner.Scan() {
		// "hierarchy-ID:controllers:path"
		fields := strings.SplitN(scanner.Text(), ":", 3)
		if len(fields) != 3 {
			continue
		}
		switch {
		case fields[0] == "0" && fields[1] == "":
			v2 = fields[2]
		case !found1 && strings.Contains(","+fields[1]+",", ",memory,"):
			v1, found1 = fields[2], true
		}
	}
	if v2 != "" {
		if limit, found := lowestLimit(root, v2, "memory.max"); found {
			return limit
		}
	}
	if found1 {
		limit, _ := lowestLimit(filepath.Join(root, "memory"), v1, "memory.limit_in_bytes")
		return limit
	}
	return 0
}

// unlimited is the least memory.limit_in_bytes that cgroup v1 writes for
// no limit: the largest number of pages, near 2^63.
const unlimited = 1 << 62

// lowestLimit is the lowest limit the files name in the cgroup at path
// under root and in its parents, 0 for none, and whether any such file
// was read. A path missing under root, as in a container that sees its
// own cgroup alone at the root, counts from its first parent there.
func lowestLimit(root, cgroup, name string) (int64, bool) {
	var lowest int64
	found := false
	for dir := path.Clean("/" + cgroup); ; dir = path.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), name))
		if err == nil {
			found = true
			text := strings.TrimSpace(string(data))
			if limit, err := strconv.ParseInt(text, 10, 64); err == nil && limit > 0 && limit < unlimited && (lowest == 0 || limit < lowest) {
				lowest = limit
			}
		}
		if dir == "/" {
			return lowest, found
		}
	}
}
