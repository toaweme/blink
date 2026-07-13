package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// nodePackageManagers is the set of package managers blink knows how to drive,
// used to validate NodeConfig.PackageManager at load time. The empty string is
// handled separately (it means auto-detect from the lockfile).
var nodePackageManagers = map[string]struct{}{
	"npm":  {},
	"pnpm": {},
	"yarn": {},
	"bun":  {},
}

// ValidPackageManager reports whether pm is a package manager blink recognizes.
// The empty string is valid and means auto-detect from the lockfile.
func ValidPackageManager(pm string) bool {
	if pm == "" {
		return true
	}
	_, ok := nodePackageManagers[pm]
	return ok
}

// nodeSelfReloadTokens are markers in a dev-script command that identify a tool
// running its own live reload / HMR (dev servers and process watchers alike).
// When a service's dev command carries one, blink must not restart the process
// on source edits: the tool already refreshes in place, so a full respawn only
// throws away its warm state. The generic --watch flags catch `node --watch`
// and any runner put in watch mode without a name blink recognizes.
var nodeSelfReloadTokens = []string{
	"vite", "next", "nuxt", "astro", "remix", "svelte-kit", "sveltekit",
	"parcel", "rsbuild", "rspack", "webpack-dev-server", "webpack serve",
	"nodemon", "tsx watch", "ts-node-dev", "tsnd", "wrangler dev",
	"--watch", "--watch-path",
}

// NodeDevCommandSelfReloads reports whether a package.json dev-script command
// runs a tool that owns its own live reload (vite, next, nodemon, node --watch,
// ...). When true, blink scopes the watch to out-of-scope files (package.json,
// node_modules) instead of restarting the dev server on every source edit.
func NodeDevCommandSelfReloads(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, tok := range nodeSelfReloadTokens {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

// NodeDevScriptSelfReloads reads dir/package.json, resolves script (defaulting
// to "dev"), and reports NodeDevCommandSelfReloads on its command. A missing
// file or script yields false, keeping the full source-restart default for
// plain node servers. Shared so the detector and the node runtime never drift.
func NodeDevScriptSelfReloads(dir, script string) bool {
	if script == "" {
		script = "dev"
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	return NodeDevCommandSelfReloads(pkg.Scripts[script])
}
