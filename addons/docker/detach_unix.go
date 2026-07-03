//go:build !windows

package docker

import (
	"os/exec"
	"syscall"
)

// detach puts a docker CLI subprocess in its own process group so a terminal
// Ctrl-C, which the kernel delivers to blink's whole foreground process group,
// never reaches it directly. Without this an in-flight `docker compose up` also
// receives the SIGINT and Compose gracefully tears the stack down, defeating the
// stop_on_exit=false default. Isolated, the stack is governed only by blink's
// own Stop path, which leaves it running unless stop_on_exit is set.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
