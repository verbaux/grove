// Package ports deterministically maps worktree aliases to TCP ports.
package ports

import (
	"errors"
	"hash/fnv"
)

// ForAlias returns a deterministic port for alias in [min, max].
func ForAlias(alias string, min, max int) int {
	h := fnv.New32a()
	h.Write([]byte(alias))
	span := max - min + 1
	return min + int(h.Sum32()%uint32(span))
}

// Allocate returns a port for alias within [min, max], avoiding any port in used.
// Starts from ForAlias(alias) and linearly probes forward, wrapping around.
// Returns error if range is invalid or fully occupied.
func Allocate(alias string, min, max int, used map[int]bool) (int, error) {
	if min <= 0 || max <= 0 || min > max {
		return 0, errors.New("invalid port range")
	}
	span := max - min + 1
	start := ForAlias(alias, min, max)
	for i := 0; i < span; i++ {
		p := min + ((start-min)+i)%span
		if !used[p] {
			return p, nil
		}
	}
	return 0, errors.New("no free ports in range")
}
