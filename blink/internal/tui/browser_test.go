package tui

import (
	"errors"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func Test_BrowserCommand(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		wantName string
		wantArgs []string
		wantErr  error
	}{
		{"macOS", "darwin", "open", []string{"http://localhost:8080"}, nil},
		{"Windows", "windows", "rundll32", []string{"url.dll,FileProtocolHandler", "http://localhost:8080"}, nil},
		{"Linux", "linux", "xdg-open", []string{"http://localhost:8080"}, nil},
		{"FreeBSD", "freebsd", "xdg-open", []string{"http://localhost:8080"}, nil},
		{"unsupported", "plan9", "", nil, errBrowserPlatformUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, err := browserCommand(tt.goos, "http://localhost:8080")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("browserCommand() error = %v, want %v", err, tt.wantErr)
			}
			if name != tt.wantName || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("browserCommand() = %q, %v, want %q, %v", name, args, tt.wantName, tt.wantArgs)
			}
		})
	}
}

type recordingBrowser struct {
	url     string
	err     error
	path    string
	command string
}

var _ externalOpener = (*recordingBrowser)(nil)

func (b *recordingBrowser) OpenBrowser(url string) error {
	b.url = url
	return b.err
}

func (b *recordingBrowser) OpenFileManager(path string) error {
	b.path = path
	return b.err
}

func (b *recordingBrowser) OpenIDE(command, path string) error {
	b.command = command
	b.path = path
	return b.err
}

func Test_Model_OpenBrowser(t *testing.T) {
	browser := &recordingBrowser{}
	m := NewModel([]string{"web"}, nil)
	m.opener = browser
	m.ports = map[string][]int{"web": {8080, 9090}}

	_, cmd := m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatalf("open browser key returned no command")
	}
	msg := cmd().(browserOpenedMsg)
	if msg.err != nil {
		t.Fatalf("open browser command returned error: %v", msg.err)
	}
	if browser.url != "http://localhost:8080" {
		t.Fatalf("opened URL = %q, want http://localhost:8080", browser.url)
	}
}

func Test_Model_OpenBrowser_NoPort(t *testing.T) {
	m := NewModel([]string{"web"}, nil)
	_, cmd := m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd != nil {
		t.Fatalf("open browser without a port returned a command")
	}
	if m.flash != "NO PORT" {
		t.Fatalf("flash = %q, want NO PORT", m.flash)
	}
}
