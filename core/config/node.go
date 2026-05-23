package config

import (
	"os"
	"path/filepath"
)

// nodeLockfiles maps a lockfile to its package manager, in detection priority
// order: pnpm is the most opinionated about its lockfile being authoritative,
// then bun (both the modern text bun.lock and the legacy binary bun.lockb),
// then yarn, finally npm.
var nodeLockfiles = []struct {
	file string
	pm   string
}{
	{"pnpm-lock.yaml", "pnpm"},
	{"bun.lock", "bun"},
	{"bun.lockb", "bun"},
	{"yarn.lock", "yarn"},
	{"package-lock.json", "npm"},
}

// NodePackageManager returns the package manager implied by the first lockfile
// present in dir, falling back to "npm" (what Node ships with). It is shared by
// the node detector and the node runtime so the two never drift.
func NodePackageManager(dir string) string {
	for _, p := range nodeLockfiles {
		if _, err := os.Stat(filepath.Join(dir, p.file)); err == nil {
			return p.pm
		}
	}
	return "npm"
}
