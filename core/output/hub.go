// Package output provides the Hub, a fan-out broker that delivers a
// supervisor's status events and captured log lines to independently
// subscribed consumers (TUI, plain UI, headless writers, remote mirrors).
package output

import (
	"sync"

	"github.com/toaweme/log"

	"github.com/toaweme/blink/core/protocol"
)

// Hub is the supervisor's fan-out event bus, one per running supervisor. Every
// consumer (TUI, plain UI, headless log writer, remote-mirror publisher) gets
// its own Subscription with independent channels, so a slow consumer cannot
// block any other.
//
// Per-subscriber channels are large but bounded. When a subscriber falls
// behind, publishes drop the event rather than block the supervisor: the next
// status resyncs the dots, and dropped log lines stay in the runner's in-memory
// Buffer so a manual refresh recovers them.
type Hub struct {
	mu      sync.RWMutex
	subs    map[*subscriber]struct{}
	closed  bool
	bufStat int
	bufLog  int
}

// Subscription is one consumer's view of the bus. Status and Logs are closed
// when the supervisor stops or the subscription is canceled, so a typical
// consumer is two goroutines each ranging over one channel.
type Subscription struct {
	Status <-chan protocol.StatusEvent
	Logs   <-chan protocol.LogLine
}

type subscriber struct {
	status chan protocol.StatusEvent
	logs   chan protocol.LogLine
}

const (
	defaultStatusBuf = 1024
	defaultLogBuf    = 4096
)

// NewHub builds a Hub with the default per-subscriber buffer sizes.
func NewHub() *Hub {
	return &Hub{
		subs:    make(map[*subscriber]struct{}),
		bufStat: defaultStatusBuf,
		bufLog:  defaultLogBuf,
	}
}

// Subscribe registers a new consumer. The returned cancel func unregisters
// the subscriber and closes its channels; the consumer's range loops exit
// naturally on channel close. Cancel is idempotent.
func (h *Hub) Subscribe() (Subscription, func()) {
	sub := &subscriber{
		status: make(chan protocol.StatusEvent, h.bufStat),
		logs:   make(chan protocol.LogLine, h.bufLog),
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(sub.status)
		close(sub.logs)
		return Subscription{Status: sub.status, Logs: sub.logs}, func() {}
	}
	h.subs[sub] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			_, ok := h.subs[sub]
			delete(h.subs, sub)
			h.mu.Unlock()
			if ok {
				close(sub.status)
				close(sub.logs)
			}
		})
	}
	return Subscription{Status: sub.status, Logs: sub.logs}, cancel
}

// PublishStatus delivers a status event to every active subscriber, dropping
// with a warning on a full per-subscriber buffer.
func (h *Hub) PublishStatus(ev protocol.StatusEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return
	}
	for sub := range h.subs {
		select {
		case sub.status <- ev:
		default:
			log.Warn("hub: dropped status event - subscriber lagging", "service", ev.Service, "status", ev.Status)
		}
	}
}

// PublishLog delivers a log line to every active subscriber, dropping silently
// on a full per-subscriber buffer (a per-line warning would itself flood).
func (h *Hub) PublishLog(ln protocol.LogLine) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return
	}
	for sub := range h.subs {
		select {
		case sub.logs <- ln:
		default:
		}
	}
}

// Close marks the hub closed and closes every subscriber's channels.
// Subsequent Publish* calls are no-ops; subsequent Subscribe returns an
// already-closed Subscription. Idempotent.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for sub := range h.subs {
		close(sub.status)
		close(sub.logs)
	}
	h.subs = nil
}
