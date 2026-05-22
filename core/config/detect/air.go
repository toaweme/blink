package detect

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/toaweme/blink/core/config"
)

// airDetector recognizes cosmtrek/air config files. A directory may hold more
// than one (.air.toml, .air.registry.toml, ...), each describing a separate
// process, so every matching file becomes its own service with reload enabled.
//
// When air's `cmd` is a recognizable `go build`, the service is emitted as a
// native `go` runtime (package from the build command, args from args_bin) so
// it gets go.work workspace watching. Any other command falls back to a shell
// service.
type airDetector struct{}

var _ Detector = airDetector{}

func (airDetector) Name() string { return "air" }

// airConfig is the subset of an air toml we map onto a blink service.
type airConfig struct {
	Root   string   `toml:"root"`
	TmpDir string   `toml:"tmp_dir"`
	Build  airBuild `toml:"build"`
}

type airBuild struct {
	Cmd        string   `toml:"cmd"`
	Bin        string   `toml:"bin"`
	FullBin    string   `toml:"full_bin"`
	ArgsBin    []string `toml:"args_bin"`
	IncludeExt []string `toml:"include_ext"`
	ExcludeDir []string `toml:"exclude_dir"`
	// ExcludeRegex is parsed but not mapped: blink's Fs.Exclude is glob-matched,
	// not regex-matched, so a regex cannot be carried across. See excludeGlobs.
	ExcludeRegex []string `toml:"exclude_regex"`
}

func (airDetector) Detect(dir string) ([]Detected, error) {
	matches, err := filepath.Glob(filepath.Join(dir, ".air*.toml"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob air configs in %s: %w", dir, err)
	}
	sort.Strings(matches)

	var out []Detected
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", filepath.Base(path), err)
		}
		var ac airConfig
		if err := toml.Unmarshal(data, &ac); err != nil {
			return nil, fmt.Errorf("failed to parse air config %s: %w", filepath.Base(path), err)
		}

		base := filepath.Base(path)
		name := airServiceName(base, dir)
		svc := config.Service{
			Name:   name,
			Reload: config.Reload{Reload: true},
			Fs: config.Fs{
				Extensions: stripDots(ac.Build.IncludeExt),
				Exclude:    excludeGlobs(ac.Build.ExcludeDir),
			},
		}

		label := name + " (air)"
		if pkg, ok := parseGoBuild(ac.Build.Cmd); ok {
			// native go runtime: args_bin becomes the go runtime's args.
			svc.Runtime = "go"
			svc.Go = &config.GoConfig{Package: pkg, Args: ac.Build.ArgsBin}
			label = name + " (air · go " + pkg + ")"
		} else {
			// not a go build: keep air's exact build+run as a shell service.
			svc.Runtime = "shell"
			if ac.Build.Cmd != "" {
				svc.Commands.Build = &config.Command{Command: ac.Build.Cmd}
			}
			if run := airRunCommand(ac.Build); run != "" {
				svc.Commands.Run = &config.Command{Command: run, Service: true}
			}
		}

		out = append(out, Detected{
			Service: svc,
			Source:  "air",
			File:    base,
			Label:   label,
		})
	}
	return out, nil
}

// parseGoBuild extracts the Go package path from an air `cmd`, returning it as
// a root-relative path ("." or "./cmd/v2/registry"). ok is false when the
// command is not a recognizable `go build`. A leading `cd <dir> &&` is handled
// by resolving the package relative to that dir.
func parseGoBuild(cmd string) (string, bool) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", false
	}

	cdDir := ""
	build := ""
	for _, seg := range strings.Split(cmd, "&&") {
		seg = strings.TrimSpace(seg)
		switch {
		case strings.HasPrefix(seg, "cd "):
			cdDir = strings.TrimSpace(strings.TrimPrefix(seg, "cd "))
		case strings.Contains(seg, "go build"):
			build = seg
		}
	}
	if build == "" {
		return "", false
	}

	pkg, ok := goBuildPackage(build)
	if !ok {
		return "", false
	}
	return joinPkg(cdDir, pkg), true
}

// goBuildPackage pulls the package argument out of a `go build ...` segment: the
// last non-flag token, skipping the `-o <out>` pair. Defaults to "." when no
// explicit package is given.
func goBuildPackage(build string) (string, bool) {
	idx := strings.Index(build, "go build")
	if idx < 0 {
		return "", false
	}
	pkg := "."
	fields := strings.Fields(build[idx+len("go build"):])
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "-o" {
			i++ // skip the output path that follows
			continue
		}
		if strings.HasPrefix(f, "-") {
			continue // any other flag (e.g. -ldflags=...)
		}
		pkg = f // last non-flag token wins
	}
	return pkg, true
}

// joinPkg resolves a package path that was written relative to a `cd` dir back
// to a root-relative "./..." form (or "." for the root package).
func joinPkg(cdDir, pkg string) string {
	if cdDir != "" {
		pkg = path.Join(cdDir, pkg)
	}
	pkg = path.Clean(strings.TrimPrefix(pkg, "./"))
	if pkg == "." || pkg == "" {
		return "."
	}
	return "./" + pkg
}

// airServiceName derives a service name from the air filename. ".air.toml"
// uses the directory basename (falling back to "app"); ".air.<suffix>.toml"
// uses <suffix>, so ".air.registry.toml" -> "registry".
func airServiceName(base, dir string) string {
	mid := strings.TrimSuffix(strings.TrimPrefix(base, ".air"), ".toml")
	mid = strings.TrimPrefix(mid, ".")
	if mid != "" {
		return mid
	}
	if b := filepath.Base(dir); b != "" && b != "." && b != string(filepath.Separator) {
		return b
	}
	return "app"
}

// airRunCommand builds the run command from air's build section: full_bin
// wins outright; otherwise bin is joined with any args_bin.
func airRunCommand(b airBuild) string {
	if b.FullBin != "" {
		return b.FullBin
	}
	if b.Bin == "" {
		return ""
	}
	parts := append([]string{b.Bin}, b.ArgsBin...)
	return strings.Join(parts, " ")
}

// excludeGlobs converts air's exclude_dir names into blink's path-glob
// convention: each name becomes "**/<name>/**" to exclude that directory
// anywhere in the tree. Entries blink already excludes by default are dropped.
func excludeGlobs(dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.Trim(d, "/")
		if d == "" || coveredByDefaultExclude(d) {
			continue
		}
		out = append(out, "**/"+d+"/**")
	}
	return out
}

// coveredByDefaultExclude reports whether dir's leaf is one blink already
// excludes from every watcher, so re-emitting it adds nothing.
func coveredByDefaultExclude(dir string) bool {
	leaf := path.Base(filepath.ToSlash(dir))
	for _, d := range config.DefaultExcludeDirs {
		if leaf == d {
			return true
		}
	}
	return false
}

// stripDots normalises air's include_ext (which may carry leading dots, e.g.
// ".go") to blink's bare-extension convention ("go").
func stripDots(exts []string) []string {
	if len(exts) == 0 {
		return nil
	}
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		out = append(out, strings.TrimPrefix(e, "."))
	}
	return out
}
