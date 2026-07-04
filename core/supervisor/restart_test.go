package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/toaweme/blink/core/addon"
	"github.com/toaweme/blink/core/config"
)

// runs reports how many times the one-off command has executed, by counting the
// bytes it appended to the counter file.
func runs(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read counter: %v", err)
	}
	return len(b)
}

// waitRuns blocks until the counter reaches want or the deadline elapses,
// returning the last observed value so the caller can report a shortfall.
func waitRuns(t *testing.T, path string, want int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := runs(t, path)
		if got >= want || time.Now().After(deadline) {
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// waitStatus blocks until the service reaches want or the deadline elapses,
// returning the last observed status.
func waitStatus(t *testing.T, s *Supervisor, name string, want Status, timeout time.Duration) Status {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := s.Status(name)
		if got == want || time.Now().After(deadline) {
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Test_Restart_OneOff verifies that pressing restart on a one-off service that
// already finished re-runs its command every time. Each run appends a byte to a
// counter file, so N restarts must yield N+1 bytes.
func Test_Restart_OneOff(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "counter")

	cfg := config.Config{
		DirRoot: dir,
		Services: []config.Service{{
			Name: "job",
			Commands: config.Commands{
				Run: &config.Command{Command: "printf x >> " + counter},
			},
		}},
	}
	reg := addon.NewRegistry()
	reg.AddRuntime(stubRuntime{})
	s, err := New(cfg, reg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	// the initial boot runs the one-off once and settles on exited.
	if got := waitRuns(t, counter, 1, 2*time.Second); got != 1 {
		t.Fatalf("after boot: runs = %d, want 1", got)
	}
	if got := waitStatus(t, s, "job", StatusExited, 2*time.Second); got != StatusExited {
		t.Fatalf("after boot: status = %q, want %q", got, StatusExited)
	}

	const restarts = 30
	for i := 1; i <= restarts; i++ {
		want := i + 1
		if err := s.Restart("job"); err != nil {
			t.Fatalf("restart %d: %v", i, err)
		}
		if got := waitRuns(t, counter, want, 2*time.Second); got != want {
			t.Fatalf("restart %d did not re-run the command: runs = %d, want %d", i, got, want)
		}
		// the re-run must settle back to exited, not clobbered by a stale
		// goroutine or left dangling.
		if got := waitStatus(t, s, "job", StatusExited, 2*time.Second); got != StatusExited {
			t.Fatalf("restart %d: status = %q, want %q", i, got, StatusExited)
		}
	}
}

// Test_Restart_ClearsRunnerOnExit verifies a finished run drops its runner slot,
// so the next restart's stopRunner has nothing to signal (no SIGINT/SIGKILL at a
// dead, possibly pid-reused process group).
func Test_Restart_ClearsRunnerOnExit(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DirRoot: dir,
		Services: []config.Service{{
			Name:     "job",
			Commands: config.Commands{Run: &config.Command{Command: "true"}},
		}},
	}
	reg := addon.NewRegistry()
	reg.AddRuntime(stubRuntime{})
	s, err := New(cfg, reg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	if got := waitStatus(t, s, "job", StatusExited, 2*time.Second); got != StatusExited {
		t.Fatalf("status = %q, want %q", got, StatusExited)
	}
	// the runner slot must be cleared once the process is gone; poll briefly
	// since the clear happens in the run goroutine just after status flips.
	deadline := time.Now().Add(time.Second)
	for s.Runner("job") != nil && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if r := s.Runner("job"); r != nil {
		t.Fatalf("Runner(job) = %p after exit, want nil (stale slot signals a dead pgid on the next restart)", r)
	}
}

// Test_Restart_NoStaleClobber restarts a still-running service and asserts the
// killed run's goroutine never clobbers the fresh run's status: the service must
// stay running (or transiently restarting), never crashed/exited/stopped, until
// we deliberately stop it. Exercises the superseded gate.
func Test_Restart_NoStaleClobber(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DirRoot: dir,
		Services: []config.Service{{
			Name: "srv",
			Commands: config.Commands{
				Run: &config.Command{Command: "sleep 5", Service: true},
			},
		}},
	}
	reg := addon.NewRegistry()
	reg.AddRuntime(stubRuntime{})
	s, err := New(cfg, reg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Stop(context.Background()) }()

	if got := waitStatus(t, s, "srv", StatusRunning, 2*time.Second); got != StatusRunning {
		t.Fatalf("boot status = %q, want %q", got, StatusRunning)
	}

	for i := 1; i <= 10; i++ {
		if err := s.Restart("srv"); err != nil {
			t.Fatalf("restart %d: %v", i, err)
		}
		// watch the window where the killed run's goroutine could still clobber
		// the fresh run's status. Only restarting/running are legitimate here.
		deadline := time.Now().Add(150 * time.Millisecond)
		sawRunning := false
		for time.Now().Before(deadline) {
			switch got := s.Status("srv"); got {
			case StatusRunning:
				sawRunning = true
			case StatusRestarting:
			default:
				t.Fatalf("restart %d: status clobbered to %q by a stale goroutine", i, got)
			}
			time.Sleep(1 * time.Millisecond)
		}
		if !sawRunning {
			if got := waitStatus(t, s, "srv", StatusRunning, time.Second); got != StatusRunning {
				t.Fatalf("restart %d: never reached running, last = %q", i, got)
			}
		}
	}
}
