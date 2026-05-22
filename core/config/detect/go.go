package detect

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/toaweme/blink/core/config"
)

// goDetector recognizes a Go module (go.mod) and emits one go-runtime service
// per main package, found by scanning for `package main` + `func main(` and
// named after the entrypoint directory. A module with no discoverable main
// falls back to a single root-package service.
type goDetector struct{}

var _ Detector = goDetector{}

func (goDetector) Name() string { return "go" }

func (goDetector) Detect(dir string) ([]Detected, error) {
	modPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(modPath); err != nil {
		// no go.mod: not a Go module, so no match.
		return nil, nil //nolint:nilerr // absence of go.mod is a no-match, not a failure
	}

	mains := findMainPackages(dir)
	if len(mains) == 0 {
		// no runnable entrypoint found: hand back a root service to refine.
		name := moduleName(modPath)
		if name == "" {
			name = filepath.Base(dir)
		}
		return []Detected{goService(name, ".", "go.mod")}, nil
	}

	var out []Detected
	for _, pkg := range mains {
		name := goServiceName(pkg, dir, modPath)
		file := "go.mod"
		if pkg != "." {
			file = filepath.ToSlash(filepath.Join(strings.TrimPrefix(pkg, "./"), "main.go"))
		}
		out = append(out, goService(name, pkg, file))
	}
	return out, nil
}

func goService(name, pkg, file string) Detected {
	return Detected{
		Service: config.Service{
			Name:    name,
			Runtime: "go",
			Go:      &config.GoConfig{Package: pkg},
			Fs:      config.Fs{Extensions: []string{"go"}},
			Reload:  config.Reload{Reload: true},
		},
		Source: "go",
		File:   file,
		Label:  name + " (go " + pkg + ")",
	}
}

// goServiceName names a main-package service: the root package borrows the
// module name; a sub-package uses its directory basename (./cmd/api -> "api").
func goServiceName(pkg, dir, modPath string) string {
	if pkg == "." {
		if name := moduleName(modPath); name != "" {
			return name
		}
		return filepath.Base(dir)
	}
	return filepath.Base(pkg)
}

// findMainPackages returns the relative package paths ("." or "./cmd/api") of
// every directory under dir that declares package main with a func main,
// skipping vendored, hidden, and build-output trees.
func findMainPackages(dir string) []string {
	var pkgs []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// skip an unreadable entry rather than aborting the walk.
			return nil //nolint:nilerr // one bad entry must not stop package discovery
		}
		if d.IsDir() {
			if path != dir && skipGoDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		if !isMainFile(path) {
			return nil
		}
		rel, err := filepath.Rel(dir, filepath.Dir(path))
		if err != nil {
			// path not relativisable against dir: skip the file, keep walking.
			return nil //nolint:nilerr // a non-relativisable path is skipped, not fatal
		}
		pkgs = appendUniqueString(pkgs, relPackage(rel))
		return nil
	})
	return pkgs
}

// relPackage formats a filepath.Rel result as a Go package path: "." for the
// root, "./sub/dir" (forward slashes) otherwise.
func relPackage(rel string) string {
	if rel == "." {
		return "."
	}
	return "./" + filepath.ToSlash(rel)
}

func skipGoDir(name string) bool {
	switch name {
	case "vendor", "node_modules", ".git", "testdata", "build", "dist", "bin", "tmp":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// isMainFile reports whether a .go file declares `package main` and contains a
// `func main(`. It reads the file once with a scanner so it stays cheap.
func isMainFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	pkgMain, funcMain := false, false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "package "):
			pkgMain = strings.TrimSpace(strings.TrimPrefix(line, "package ")) == "main"
		case strings.HasPrefix(line, "func main("):
			funcMain = true
		}
		if pkgMain && funcMain {
			return true
		}
	}
	return false
}

func appendUniqueString(s []string, v string) []string {
	for _, e := range s {
		if e == v {
			return s
		}
	}
	return append(s, v)
}

// moduleName reads the module path from a go.mod and returns its last path
// segment, e.g. "github.com/toaweme/blink" -> "blink". Returns "" when the
// module line is missing or unreadable.
func moduleName(modPath string) string {
	f, err := os.Open(modPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		mod := strings.TrimSpace(strings.TrimPrefix(line, "module "))
		mod = strings.Trim(mod, `"`)
		if mod == "" {
			return ""
		}
		return filepath.Base(mod)
	}
	return ""
}
