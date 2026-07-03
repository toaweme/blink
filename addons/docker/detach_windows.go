//go:build windows

package docker

import "os/exec"

// detach is a no-op on Windows, which has no process-group signaling and does
// not deliver a console Ctrl-C to child processes the way a unix terminal
// signals its foreground group. See detach_unix.go for the rationale.
func detach(cmd *exec.Cmd) {}
