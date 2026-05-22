package ui

import (
	"sync"

	"github.com/toaweme/log"
)

var (
	slogMu   sync.Mutex
	slogPrev log.Logger
)

// silenceSlog replaces the global logger with a discard one; restoreSlog
// restores the previous logger.
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
