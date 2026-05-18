package exec

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/toaweme/log"
)

// Runner owns a single OS process.
type Runner struct {
	config Config
	cmd    *exec.Cmd

	mu        sync.Mutex
	startedAt time.Time

	// outputBuf holds every captured line - stdout and stderr are merged at
	// the pipe level so a single reader sees them in source order.
	outputBuf *Buffer

	// tee is an optional writer that receives every captured line with a
	// trailing newline. The TUI uses it to mirror output live.
	tee io.Writer

	// stdin is the parent end of the child's stdin pipe. Populated only when
	// Config.Stdin is true; nil otherwise so existing services that inherit
	// the parent's stdin keep their current behavior.
	stdin io.WriteCloser

	// done is closed by Run() after cmd.Wait() returns and pipes are reaped.
	// Stop waits on this so callers don't race the next bind against a still-
	// dying process.
	done chan struct{}
}

type Config struct {
	// Name identifies the owning service in logs.
	Name    string
	Dir     string
	Command string
	Args    []string
	Env     map[string]string
	// Stdin, when true, gives the runner its own stdin pipe instead of
	// inheriting the parent's. Required for `blink exec <target> stdin` to
	// deliver bytes to the child. Opt-in so default-mode services keep their
	// current behavior (inherited stdin, typing in the host terminal works).
	Stdin bool
}

func (c Config) Environment() []string {
	env := make([]string, 0, len(c.Env))
	for k, v := range c.Env {
		env = append(env, k+"="+v)
	}
	return env
}

func NewRunner(cfg Config) *Runner {
	return &Runner{
		config:    cfg,
		outputBuf: NewBuffer(),
		done:      make(chan struct{}),
	}
}

// SetTee installs a writer that mirrors all captured output. Pass nil to detach.
func (r *Runner) SetTee(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tee = w
}

// Run starts the command and blocks until it exits. done is closed before
// return so Stop callers can wait on full process teardown.
func (r *Runner) Run() error {
	defer close(r.done)

	cmd, output, err := r.start(r.config)
	if err != nil {
		return fmt.Errorf("failed to start command %q: %w", r.config.Command, err)
	}

	r.mu.Lock()
	r.cmd = cmd
	r.startedAt = time.Now()
	r.mu.Unlock()

	// single goroutine reads the merged stdout+stderr pipe so lines stay in
	// the order the child wrote them (panic stacks remain readable).
	captureDone := make(chan struct{})
	go func() {
		defer close(captureDone)
		r.captureOutput(output, r.outputBuf)
	}()

	log.Info("process started", "service", r.config.Name, "pid", cmd.Process.Pid, "command", r.config.Command)
	waitErr := cmd.Wait()
	// drain pipe before returning so done callers don't race late writes.
	<-captureDone
	if waitErr != nil {
		return waitErr
	}
	return nil
}

// Done returns a channel closed when Run() has returned (process reaped,
// pipes drained). Multiple callers may receive on it.
func (r *Runner) Done() <-chan struct{} { return r.done }

func (r *Runner) captureOutput(src io.ReadCloser, buf *Buffer) {
	defer src.Close()
	// bufio.Reader (not Scanner) so a single huge line cannot poison the stream
	// - Scanner aborts permanently on ErrTooLong, dropping every subsequent line.
	reader := bufio.NewReaderSize(src, 64*1024)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			text := strings.TrimRight(line, "\n")
			buf.Append(text)

			r.mu.Lock()
			tee := r.tee
			r.mu.Unlock()
			if tee != nil {
				_, _ = tee.Write([]byte(text + "\n"))
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Warn("failed to read process output", "service", r.config.Name, "error", err)
			}
			return
		}
	}
}

// Output returns the merged stdout+stderr buffer.
func (r *Runner) Output() *Buffer { return r.outputBuf }

// StartedAt is the time of the most recent Run() invocation that succeeded in
// launching a process. Zero value means the runner has not started yet.
func (r *Runner) StartedAt() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startedAt
}

// Pid returns the OS pid of the running process, or 0 if not running.
func (r *Runner) Pid() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil || r.cmd.Process == nil {
		return 0
	}
	return r.cmd.Process.Pid
}

// Stop signals the process group with SIGINT, gives it up to graceTimeout
// to exit, then SIGKILLs the group. Blocks until Run() returns so callers
// don't race the next bind against a still-dying process.
func (r *Runner) Stop() error {
	return r.StopWithGrace(true, 2*time.Second)
}

func (r *Runner) StopWithGrace(interrupt bool, graceTimeout time.Duration) error {
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	log.Info("process stopping", "service", r.config.Name, "pid", cmd.Process.Pid)
	_, err := r.kill(cmd, interrupt, graceTimeout)
	return err
}

// Write delivers bytes to the child's stdin. Returns an error if the runner
// was not configured with Config.Stdin or if the process has exited and the
// pipe has been closed.
func (r *Runner) Write(p []byte) (int, error) {
	r.mu.Lock()
	w := r.stdin
	r.mu.Unlock()
	if w == nil {
		return 0, fmt.Errorf("service %q: stdin not enabled", r.config.Name)
	}
	return w.Write(p)
}

// CloseStdin closes the child's stdin. Useful for commands that read until EOF.
func (r *Runner) CloseStdin() error {
	r.mu.Lock()
	w := r.stdin
	r.stdin = nil
	r.mu.Unlock()
	if w == nil {
		return nil
	}
	return w.Close()
}

func (r *Runner) Signal(sig os.Signal) error {
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(sig)
}
