package docker

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"

	"github.com/toaweme/log"
)

// runLogStream tails one compose service's logs and pipes each line into the
// per-child log channel for the TUI/plain UI to consume.
func (m *Manager) runLogStream(ctx context.Context, child string) {
	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-p", m.project, "-f", m.composeFile,
		"logs", "-f", "--no-color", "--no-log-prefix", child,
	)
	cmd.Dir = m.workDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Warn("docker logs: stdout pipe", "child", child, "error", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Warn("docker logs: stderr pipe", "child", child, "error", err)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Warn("docker logs: start", "child", child, "error", err)
		return
	}
	defer func() { _ = cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()

	go m.scanLines(ctx, child, stderr)
	m.scanLines(ctx, child, stdout)
}

func (m *Manager) scanLines(ctx context.Context, child string, r interface{ Read(p []byte) (int, error) }) {
	reader := bufio.NewReaderSize(r, 64*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			m.emitLog(child, strings.TrimRight(line, "\n"))
		}
		if err != nil {
			if err != io.EOF {
				log.Warn("docker logs read", "child", child, "error", err)
			}
			return
		}
	}
}
