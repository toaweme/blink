package control

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// ParseSignal accepts common signal names (with or without the SIG
// prefix, case-insensitive) and returns the matching os.Signal. Used by
// the default Signal verb handler.
func ParseSignal(name string) (os.Signal, error) {
	n := strings.ToUpper(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "SIG")
	switch n {
	case "INT":
		return syscall.SIGINT, nil
	case "TERM":
		return syscall.SIGTERM, nil
	case "KILL":
		return syscall.SIGKILL, nil
	case "HUP":
		return syscall.SIGHUP, nil
	case "USR1":
		return syscall.SIGUSR1, nil
	case "USR2":
		return syscall.SIGUSR2, nil
	case "QUIT":
		return syscall.SIGQUIT, nil
	case "":
		return nil, errors.New("missing signal name")
	default:
		return nil, fmt.Errorf("unsupported signal %q", name)
	}
}
