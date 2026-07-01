//go:build windows

package exec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/toaweme/log"
)

// kill terminates the process, then waits for Run()'s goroutine to finish
// reaping. Returns once r.done is closed (process fully torn down) so callers
// can rebind ports immediately.
//
// Windows has no SIGINT/process-group signaling equivalent to unix, so
// interrupt is ignored and the process is killed directly.
func (r *Runner) kill(cmd *exec.Cmd, interrupt bool, graceTimeout time.Duration) (pid int, err error) {
	pid = cmd.Process.Pid

	if e := cmd.Process.Kill(); e != nil && !errors.Is(e, os.ErrProcessDone) {
		log.Warn("kill failed", "service", r.config.Name, "pid", pid, "error", e)
	}
	// bound the post-kill wait so a zombie can't block restarts forever.
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		log.Warn("process did not exit after kill", "service", r.config.Name, "pid", pid)
	}
	return pid, nil
}

func (r *Runner) start(cfg Config) (*exec.Cmd, io.ReadCloser, error) {
	log.Info("executing", "service", cfg.Name, "command", cfg.Command, "dir", cfg.Dir)
	//nolint:gosec,noctx // cfg.Command is the user's own service command from blink.yaml
	c := exec.Command("cmd", "/C", cfg.Command)
	c.Dir = cfg.Dir
	c.Env = os.Environ()
	c.Env = append(c.Env, cfg.Environment()...)

	// merge stdout and stderr into one pipe so lines stay in the order the child
	// wrote them. With separate pipes a stderr panic gets interleaved with
	// stdout traces and becomes unreadable.
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create output pipe: %w", err)
	}
	c.Stdout = pw
	c.Stderr = pw

	// optional stdin pipe, only when Config.Stdin is set, so services not using
	// the control socket keep inherited-stdin behavior.
	var stdinR *os.File
	if cfg.Stdin {
		var stdinW *os.File
		stdinR, stdinW, err = os.Pipe()
		if err != nil {
			_ = pr.Close()
			_ = pw.Close()
			return nil, nil, fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		c.Stdin = stdinR
		r.mu.Lock()
		r.stdin = stdinW
		r.mu.Unlock()
	}

	if err := c.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		if stdinR != nil {
			_ = stdinR.Close()
		}
		return nil, nil, fmt.Errorf("failed to start command %q: %w", cfg.Command, err)
	}
	// close the parent's copy of the write end so we get EOF on read once
	// the child exits.
	_ = pw.Close()
	if stdinR != nil {
		// child has its own copy of the read end now; drop the parent's so we
		// don't leak the descriptor and so EOF reaches the child cleanly when
		// the writer side closes.
		_ = stdinR.Close()
	}
	return c, pr, nil
}
