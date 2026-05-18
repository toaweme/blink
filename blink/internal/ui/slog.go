package ui

import (
	"sync"

	"github.com/toaweme/log"
)

var (
	slogMu   sync.Mutex
	slogPrev log.Logger
)

// silenceSlog replaces toaweme/log's global logger with a discard one. The
// previous logger is restored by restoreSlog().
func silenceSlog() {
	slogMu.Lock()
	defer slogMu.Unlock()
	slogPrev = log.Default()
	log.SetDefault(log.Discard())
}

func restoreSlog() {
	slogMu.Lock()
	defer slogMu.Unlock()
	if slogPrev != nil {
		log.SetDefault(slogPrev)
	}
}
