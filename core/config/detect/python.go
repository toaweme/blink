package detect

import (
	"os"
	"path/filepath"

	"github.com/toaweme/blink/core/config"
)

// pythonDetector recognizes a Python project and emits a shell-runtime service
// pointed at the most likely entrypoint: manage.py runserver, an app.py /
// main.py module, else a bare `python -m <module>` the user refines.
type pythonDetector struct{}

var _ Detector = pythonDetector{}

func (pythonDetector) Name() string { return "python" }

func (pythonDetector) Detect(dir string) ([]Detected, error) {
	name := filepath.Base(dir)

	if exists(filepath.Join(dir, "manage.py")) {
		return []Detected{pythonService(name, "python manage.py runserver", "manage.py")}, nil
	}

	if entry := pythonEntrypoint(dir); entry != "" {
		return []Detected{pythonService(name, "python "+entry, entry)}, nil
	}

	for _, marker := range []string{"pyproject.toml", "requirements.txt"} {
		if exists(filepath.Join(dir, marker)) {
			return []Detected{pythonService(name, "python -m "+sanitizeModule(name), marker)}, nil
		}
	}
	return nil, nil
}

func pythonService(name, run, file string) Detected {
	return Detected{
		Service: config.Service{
			Name:     name,
			Runtime:  "shell",
			Commands: config.Commands{Run: &config.Command{Command: run, Service: true}},
			Fs:       config.Fs{Extensions: []string{"py"}},
			Reload:   config.Reload{Reload: true},
		},
		Source: "python",
		File:   file,
		Label:  name + " (python)",
	}
}

// pythonEntrypoint returns the first conventional entry module present.
func pythonEntrypoint(dir string) string {
	for _, f := range []string{"main.py", "app.py", "wsgi.py", "asgi.py"} {
		if exists(filepath.Join(dir, f)) {
			return f
		}
	}
	return ""
}

// sanitizeModule turns a directory name into a plausible python module name
// (hyphens to underscores) for the `python -m <module>` fallback.
func sanitizeModule(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		if r == '-' || r == ' ' {
			out = append(out, '_')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
