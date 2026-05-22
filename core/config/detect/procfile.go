package detect

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/toaweme/blink/core/config"
)

// procfileDetector recognizes a Procfile and emits one shell-runtime service
// per `name: command` line, mirroring the Foreman/Heroku process model.
type procfileDetector struct{}

var _ Detector = procfileDetector{}

func (procfileDetector) Name() string { return "procfile" }

func (procfileDetector) Detect(dir string) ([]Detected, error) {
	path := filepath.Join(dir, "Procfile")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read Procfile: %w", err)
	}
	defer f.Close()

	var out []Detected
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cmd, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		cmd = strings.TrimSpace(cmd)
		if !ok || name == "" || cmd == "" {
			continue
		}
		svc := config.Service{
			Name:     name,
			Runtime:  "shell",
			Commands: config.Commands{Run: &config.Command{Command: cmd, Service: true}},
		}
		out = append(out, Detected{
			Service: svc,
			Source:  "procfile",
			File:    "Procfile",
			Label:   name + " (procfile)",
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan Procfile: %w", err)
	}
	return out, nil
}
