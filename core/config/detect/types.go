// Package detect infers blink services from the files already present in a
// project directory, so `blink run` works with no blink.yaml and `blink init`
// can pre-fill the wizard. Each Detector inspects one directory and reports
// the services it recognises; Scan runs the whole set and assembles a Config.
package detect

import "github.com/toaweme/blink/core/config"

// Detector inspects a single directory and reports the services it can infer.
// A detector that finds nothing returns (nil, nil); it only returns an error
// when the directory looked like a match but the source was unparseable (e.g.
// a malformed air toml or compose file) so the user learns their file is
// broken instead of being silently skipped.
type Detector interface {
	Name() string
	Detect(dir string) ([]Detected, error)
}

// Detected wraps an inferred service with the provenance the picker surfaces:
// which detector produced it, the file that triggered it, and a one-line
// label for the checkbox list.
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
