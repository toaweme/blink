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

// airDetector recognises cosmtrek/air config files. A directory often holds
// more than one (.air.toml, .air.registry.toml, .air.slack.toml, ...), each
// describing a separate process, so every matching file becomes its own
// service. blink owns the watch+restart loop air would normally run, so the
// air build/run commands are ported and reload is enabled.
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
	// ExcludeRegex is parsed so the field is documented, but deliberately not
	// mapped: blink's Fs.Exclude is glob-matched, not regex-matched, so a
	// regex can't be carried across faithfully. See excludeGlobs.
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
			Name:    name,
			Runtime: "shell",
			Reload:  config.Reload{Reload: true},
			Fs: config.Fs{
				Extensions: stripDots(ac.Build.IncludeExt),
				Exclude:    excludeGlobs(ac.Build.ExcludeDir),
			},
		}
		if ac.Build.Cmd != "" {
			svc.Commands.Build = &config.Command{Command: ac.Build.Cmd}
		}
		if run := airRunCommand(ac.Build); run != "" {
			svc.Commands.Run = &config.Command{Command: run, Service: true}
		}

		out = append(out, Detected{
			Service: svc,
			Source:  "air",
			File:    base,
			Label:   name + " (air)",
		})
	}
	return out, nil
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
// convention. blink matches Fs.Exclude against the full path, where a bare
// "tmp" matches nothing, so each name becomes "**/<name>/**" to exclude that
// directory anywhere in the tree. Entries blink already excludes by default
// are dropped: a default "**/<dir>/**" matches that dir at any depth, so both
// a bare "node_modules" and a nested "ui/node_modules" are redundant.
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
