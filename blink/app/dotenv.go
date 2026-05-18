package app

import (
	"path/filepath"

	"github.com/toaweme/cli"
)

// LoadDotEnv loads <cwd>/.env into the process environment before config
// resolution. It only sets variables that are not already present, so a
// value exported in the shell always wins over the file. A missing .env is
// not an error. The cli's env: tags then pick these up like any other env.
func LoadDotEnv(cwd string) error {
	return cli.LoadDotEnv(filepath.Join(cwd, ".env"))
}
