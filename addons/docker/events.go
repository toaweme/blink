package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/core/addon"
)

// dockerEvent is the consumed subset of `docker events --format '{{json .}}'`. Type=="container" events carry compose metadata in Actor.Attributes.
type dockerEvent struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
}

// composePsRow is the consumed subset of `docker compose ps --format json`.
type composePsRow struct {
	Name       string             `json:"Name"`
	Service    string             `json:"Service"`
	State      string             `json:"State"`
	Health     string             `json:"Health"`
	Publishers []composePublisher `json:"Publishers"`
}

// composePublisher describes one host->container port mapping for a service.
type composePublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// composeRows runs `docker compose ps --format json` and returns the parsed rows. Shared by seedStatus and waitForPublishedPorts.
func (m *Manager) composeRows(ctx context.Context) ([]composePsRow, error) {
	//nolint:gosec // docker CLI args are derived from validated config, not user input
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", m.project, "-f", m.composeFile, "ps", "--format", "json")
	cmd.Dir = m.workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseComposePs(out)
}

func (m *Manager) seedStatus(ctx context.Context) error {
	rows, err := m.composeRows(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		// attach this container's own published ports so a focused container tab
		// shows just its address, not the whole stack's.
		m.emit(addon.ManagerEvent{
			Child:  row.Service,
			Status: normaliseState(row.State, row.Health),
			Ports:  collectPorts([]composePsRow{row}, nil),
		})
	}
	return nil
}

// parseComposePs handles both the JSON-array form (older `docker compose`) and the NDJSON form (newer).
func parseComposePs(data []byte) ([]composePsRow, error) {
	trimmed := skipWhitespace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var rows []composePsRow
		if err := json.Unmarshal(trimmed, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	var rows []composePsRow
	dec := json.NewDecoder(bytes(trimmed))
	for dec.More() {
		var row composePsRow
		if err := dec.Decode(&row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func skipWhitespace(b []byte) []byte {
	for i, c := range b {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return b[i:]
		}
	}
	return nil
}

// byteReader provides an io.Reader over a byte slice without importing the bytes package.
type byteReader struct {
	b []byte
	i int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

func bytes(b []byte) io.Reader { return &byteReader{b: b} }

func (m *Manager) runEventStream(ctx context.Context) {
	//nolint:gosec // docker CLI args are derived from validated config, not user input
	cmd := exec.CommandContext(ctx, "docker", "events",
		"--filter", "label=com.docker.compose.project="+m.project,
		"--filter", "type=container",
		"--format", "{{json .}}",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Warn("docker events: stdout pipe", "error", err)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Warn("docker events: start", "error", err)
		return
	}
	defer func() { _ = cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()

	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Warn("docker events read", "error", err)
			}
			return
		}
		var ev dockerEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Type != "container" {
			continue
		}
		svc := ev.Actor.Attributes["com.docker.compose.service"]
		if svc == "" {
			continue
		}
		status := mapEventAction(ev.Action)
		if status == "" {
			continue
		}
		m.emit(addon.ManagerEvent{Child: svc, Status: status})
	}
}

// mapEventAction translates a docker container event verb into a supervisor status string. Returns "" for events that aren't surfaced (exec_*, top, etc).
func mapEventAction(action string) string {
	switch action {
	case "start":
		return "running"
	case "die", "kill", "stop":
		return "exited"
	case "health_status: healthy":
		return "running"
	case "health_status: unhealthy":
		return "crashed"
	case "create":
		return "building"
	case "destroy":
		return "stopped"
	}
	return ""
}

// normaliseState maps `docker compose ps` state strings to supervisor status.
func normaliseState(state, health string) string {
	switch state {
	case "running":
		if health == "unhealthy" {
			return "crashed"
		}
		return "running"
	case "exited", "dead":
		return "exited"
	case "restarting":
		return "restarting"
	case "created":
		return "building"
	}
	return state
}
