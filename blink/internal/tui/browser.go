package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/toaweme/blink/blink/internal/theme"
)

var errBrowserPlatformUnsupported = errors.New("browser platform is unsupported")

type systemExternalOpener struct {
	goos  string
	start func(name string, args ...string) error
}

var _ externalOpener = (*systemExternalOpener)(nil)

func newSystemExternalOpener() externalOpener {
	return &systemExternalOpener{
		goos: runtime.GOOS,
		start: func(name string, args ...string) error {
			cmd := exec.CommandContext(context.Background(), name, args...)
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Process.Release()
		},
	}
}

func (o *systemExternalOpener) OpenBrowser(url string) error {
	name, args, err := browserCommand(o.goos, url)
	if err != nil {
		return err
	}
	if err := o.start(name, args...); err != nil {
		return fmt.Errorf("failed to open %q in the browser: %w", url, err)
	}
	return nil
}

func (o *systemExternalOpener) OpenFileManager(path string) error {
	name, args, err := fileManagerCommand(o.goos, path)
	if err != nil {
		return err
	}
	if err := o.start(name, args...); err != nil {
		return fmt.Errorf("failed to open %q in the file manager: %w", path, err)
	}
	return nil
}

func (o *systemExternalOpener) OpenIDE(command, path string) error {
	resolved, err := resolveIDECommand(o.goos, command, exec.LookPath)
	if err != nil {
		return err
	}
	if err := o.start(resolved, path); err != nil {
		return fmt.Errorf("failed to open %q with IDE %q: %w", path, command, err)
	}
	return nil
}

func browserCommand(goos, url string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris":
		return "xdg-open", []string{url}, nil
	default:
		return "", nil, fmt.Errorf("failed to open the browser on %q: %w", goos, errBrowserPlatformUnsupported)
	}
}

func fileManagerCommand(goos, path string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{path}, nil
	case "windows":
		return "explorer.exe", []string{path}, nil
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris":
		return "xdg-open", []string{path}, nil
	default:
		return "", nil, fmt.Errorf("failed to open the file manager on %q: %w", goos, errBrowserPlatformUnsupported)
	}
}

type externalOpener interface {
	OpenBrowser(url string) error
	OpenFileManager(path string) error
	OpenIDE(command, path string) error
}

type browserOpenedMsg struct {
	url string
	err error
}

func (m *Model) openBrowser() tea.Cmd {
	ports := m.ports[m.viewKey()]
	if len(ports) == 0 {
		m.setFlash("NO PORT", theme.Warning)
		return nil
	}
	url := fmt.Sprintf("http://localhost:%d", ports[0])
	return func() tea.Msg {
		return browserOpenedMsg{url: url, err: m.opener.OpenBrowser(url)}
	}
}
