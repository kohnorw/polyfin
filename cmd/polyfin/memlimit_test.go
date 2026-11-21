package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Go's soft limit is half the container's, unless GOMEMLIMIT sets one;
// the caches keep the share of their budgets the soft limit is of 4 GiB,
// a thirty-second at least, and all of them without a limit.
func TestTheMemoryLimitFollowsTheContainer(t *testing.T) {
	t.Parallel()
	const mib = 1 << 20
	for _, tc := range []struct {
		name      string
		fromEnv   bool
		current   int64
		container int64
		want      memoryPlan
	}{
		{"no limit", false, math.MaxInt64, 0, memoryPlan{share: 1}},
		{"a 1 GiB container", false, math.MaxInt64, 1024 * mib, memoryPlan{limit: 512 * mib, set: true, share: 0.125}},
		{"a 16 GiB container", false, math.MaxInt64, 16384 * mib, memoryPlan{limit: 8192 * mib, set: true, share: 1}},
		{"a 128 MiB container", false, math.MaxInt64, 128 * mib, memoryPlan{limit: 64 * mib, set: true, share: 1.0 / 32}},
		{"GOMEMLIMIT in a 1 GiB container", true, 300 * mib, 1024 * mib, memoryPlan{limit: 300 * mib, share: 300.0 / 4096}},
		{"GOMEMLIMIT=off in a 1 GiB container", true, math.MaxInt64, 1024 * mib, memoryPlan{share: 1}},
	} {
		if got := planMemory(tc.fromEnv, tc.current, tc.container); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// The container's memory limit comes from the cgroup /proc/self/cgroup
// names, the lowest along its path: cgroup v2's memory.max, else v1's
// memory.limit_in_bytes, from fake cgroup file systems.
func TestTheContainerMemoryLimitComesFromTheCgroup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		cgroups string
		files   map[string]string
		want    int64
	}{
		{"v2 in a container", "0::/\n", map[string]string{"memory.max": "1073741824\n"}, 1 << 30},
		{"v2 without a limit", "0::/\n", map[string]string{"memory.max": "max\n"}, 0},
		{"v2 under systemd, the slice limited", "0::/system.slice/polyfin.service\n", map[string]string{
			"system.slice/memory.max":                 "2147483648\n",
			"system.slice/polyfin.service/memory.max": "max\n",
		}, 2 << 30},
		{"v1 seeing its own cgroup at the root", "12:memory:/docker/0123abcd\n4:cpu,cpuacct:/docker/0123abcd\n", map[string]string{
			"memory/memory.limit_in_bytes": "536870912\n",
		}, 512 << 20},
		{"v1 without a limit", "12:memory:/\n", map[string]string{"memory/memory.limit_in_bytes": "9223372036854771712\n"}, 0},
		{"v2 without the memory controller, v1 with it", "12:memory:/\n0::/\n", map[string]string{
			"cgroup.controllers":           "cpu io\n",
			"memory/memory.limit_in_bytes": "268435456\n",
		}, 256 << 20},
		{"no cgroup files", "0::/\n", nil, 0},
	} {
		root := t.TempDir()
		for name, content := range tc.files {
			file := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := cgroupMemoryLimit(root, tc.cgroups); got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
}
