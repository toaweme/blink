package detect

import (
	"fmt"
	"strconv"

	"github.com/toaweme/blink/core/config"
)

// Detectors returns the default detector set in priority order. Order is the
// dedup tie-break: when two detectors infer the same service name, the earlier
// one keeps the bare name and the later one is suffixed.
func Detectors() []Detector {
	return []Detector{
		goDetector{},
		airDetector{},
		pythonDetector{},
		rustDetector{},
		procfileDetector{},
	}
}

// Scan runs every detector against dir and assembles an in-memory Config ready
// to hand to the supervisor. It returns the Config alongside the flat list of
// Detected (so callers can render labels/provenance in a picker). DirRoot is
// set to dir; Paths are left zero so the caller resolves them. Service names
// are made unique (first detector wins; later collisions get a numeric
// suffix) so the result always passes loader.Validate.
func Scan(dir string) (config.Config, []Detected, error) {
	var detected []Detected
	for _, d := range Detectors() {
		found, err := d.Detect(dir)
		if err != nil {
			return config.Config{}, nil, fmt.Errorf("failed to detect %s services: %w", d.Name(), err)
		}
		detected = append(detected, found...)
	}

	detected = dropGoAirOverlap(detected)

	seen := make(map[string]struct{}, len(detected))
	for i := range detected {
		name := uniqueName(detected[i].Service.Name, seen)
		seen[name] = struct{}{}
		detected[i].Service.Name = name
	}

	cfg := config.Config{DirRoot: dir}
	for _, d := range detected {
		cfg.Services = append(cfg.Services, d.Service)
	}
	return cfg, detected, nil
}

// dropGoAirOverlap removes go-runtime services that an air config already
// covers. air is a hot-reloader for the same Go binaries, so a ".air.registry"
// service and the go service for ./cmd/registry are the same process described
// twice; the air entry wins because it carries the user's real build/run
// command, watch extensions, and excludes. Matching is by service name, which
// is how both detectors name a service after its entrypoint. Collisions between
// unrelated detectors still fall through to uniqueName's numeric suffix.
func dropGoAirOverlap(detected []Detected) []Detected {
	airNames := make(map[string]struct{})
	for _, d := range detected {
		if d.Source == "air" {
			airNames[d.Service.Name] = struct{}{}
		}
	}
	if len(airNames) == 0 {
		return detected
	}
	out := detected[:0]
	for _, d := range detected {
		if d.Source == "go" {
			if _, dup := airNames[d.Service.Name]; dup {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

// uniqueName returns name if free, else name-2, name-3, ... until one is.
func uniqueName(name string, seen map[string]struct{}) string {
	if name == "" {
		name = "service"
	}
	if _, dup := seen[name]; !dup {
		return name
	}
	for n := 2; ; n++ {
		candidate := name + "-" + strconv.Itoa(n)
		if _, dup := seen[candidate]; !dup {
			return candidate
		}
	}
}
