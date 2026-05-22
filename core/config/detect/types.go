// Package detect infers blink services from the files already present in a
// project directory, so `blink run` works with no blink.yaml and `blink init`
// can pre-fill the wizard. Each Detector inspects one directory and reports
// the services it recognizes; Scan runs the whole set and assembles a Config.
package detect

import "github.com/toaweme/blink/core/config"

// Detector inspects a single directory and reports the services it can infer.
// A detector that finds nothing returns (nil, nil); it returns an error only
// when the source looked like a match but was unparseable (e.g. a malformed air
// toml or compose file).
type Detector interface {
	Name() string
	Detect(dir string) ([]Detected, error)
}

// Detected wraps an inferred service with the provenance the picker surfaces:
// which detector produced it, the file that triggered it, and a label.
type Detected struct {
	// Service is the fully-formed service, ready to run as-is.
	Service config.Service
	// Source is the detector name, e.g. "air", "go", "node".
	Source string
	// File is the relative file that triggered detection, e.g. ".air.registry.toml".
	File string
	// Label is the human summary shown in the picker, e.g. "registry (air)".
	Label string
}
