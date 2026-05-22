package docker

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/toaweme/log"
)

// waitForPublishedPorts blocks until every TCP port published by the compose stack accepts a connection. It complements `docker compose up --wait`, which only blocks on containers that declare a healthcheck: images without one (stock mysql/postgres) report "running" before the daemon inside accepts connections. Ports are dialed in parallel with exponential backoff; returns once every port is reachable or the context cancels.
func (m *Manager) waitForPublishedPorts(ctx context.Context) error {
	rows, err := m.composeRows(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers for port probe: %w", err)
	}

	addrs := collectAddrs(rows, m.services)
	if len(addrs) == 0 {
		return nil
	}

	log.Debug("docker: probing tcp readiness", "addrs", addrs)

	var wg sync.WaitGroup
	errCh := make(chan error, len(addrs))
	for _, addr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			if err := dialUntilReady(ctx, addr); err != nil {
				errCh <- fmt.Errorf("failed to probe port %s: %w", addr, err)
			}
		}(addr)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		return err
	}
	return nil
}

// collectAddrs returns the host:port list to probe. A non-empty serviceFilter limits it to services in that set.
func collectAddrs(rows []composePsRow, serviceFilter []string) []string {
	want := make(map[string]struct{}, len(serviceFilter))
	for _, s := range serviceFilter {
		want[s] = struct{}{}
	}

	seen := make(map[string]struct{})
	var addrs []string
	for _, row := range rows {
		if len(want) > 0 {
			if _, ok := want[row.Service]; !ok {
				continue
			}
		}
		for _, pub := range row.Publishers {
			if pub.PublishedPort == 0 {
				continue
			}
			if pub.Protocol != "" && pub.Protocol != "tcp" {
				continue
			}
			host := pub.URL
			if host == "" || host == "0.0.0.0" || host == "::" {
				host = "127.0.0.1"
			}
			addr := net.JoinHostPort(host, strconv.Itoa(pub.PublishedPort))
			if _, dup := seen[addr]; dup {
				continue
			}
			seen[addr] = struct{}{}
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

// dialUntilReady opens TCP connections to addr with exponential backoff (capped at 2s) until one succeeds or ctx cancels. The wait is bounded by the service's actual boot time, not a fixed timeout.
func dialUntilReady(ctx context.Context, addr string) error {
	backoff := 100 * time.Millisecond
	const maxBackoff = 2 * time.Second
	dialer := &net.Dialer{Timeout: 1 * time.Second}

	for {
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
