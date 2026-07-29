package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func Test_IDEPreferences_Resolve(t *testing.T) {
	prefs := idePreferences{IDE: idePreferenceSet{
		Default:  "code",
		Projects: []projectIDEPreference{{Root: "/work/app", Executable: "zed", Services: map[string]string{"api": "goland"}}},
	}}
	tests := []struct{ service, want, source string }{
		{"api", "goland", "service"},
		{"web", "zed", "project"},
	}
	for _, tt := range tests {
		got, source := prefs.resolve("/work/app", tt.service)
		if got != tt.want || source != tt.source {
			t.Fatalf("Resolve(%q) = %q, %q, want %q, %q", tt.service, got, source, tt.want, tt.source)
		}
	}
	got, source := prefs.resolve("/work/other", "api")
	if got != "code" || source != "global" {
		t.Fatalf("global Resolve() = %q, %q, want code, global", got, source)
	}
}

func Test_YAMLIDEPreferenceStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := newYAMLIDEPreferenceStore(dir)
	prefs := idePreferences{IDE: idePreferenceSet{Default: "code"}}
	if err := store.Save(prefs); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if got.IDE.Default != "code" {
		t.Fatalf("default = %q, want code", got.IDE.Default)
	}
	info, err := os.Stat(filepath.Join(dir, "settings.yml"))
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("settings mode = %o, want 644", info.Mode().Perm())
	}
}

type memoryIDEStore struct {
	prefs idePreferences
	saves int
}

var _ idePreferenceStore = (*memoryIDEStore)(nil)

func (s *memoryIDEStore) Load() (idePreferences, error) { return s.prefs, nil }
func (s *memoryIDEStore) Save(p idePreferences) error   { s.prefs = p; s.saves++; return nil }

func Test_Model_OpenIDE_UsesServicePreferenceAndPath(t *testing.T) {
	opener := &recordingBrowser{}
	store := &memoryIDEStore{prefs: idePreferences{IDE: idePreferenceSet{Default: "code", Projects: []projectIDEPreference{{Root: "/work", Services: map[string]string{"api": "goland"}}}}}}
	m := NewModel([]string{"api"}, nil)
	m.projectPath = "/work"
	m.servicePaths = map[string]string{"api": "/work/services/api"}
	m.opener = opener
	m.settings = store

	_, cmd := m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	if cmd == nil {
		t.Fatalf("Open IDE returned no command")
	}
	msg := cmd().(externalOpenedMsg)
	if msg.err != nil || opener.command != "goland" || opener.path != "/work/services/api" {
		t.Fatalf("opened %q with %q, error %v", opener.path, opener.command, msg.err)
	}
	if store.saves != 0 {
		t.Fatalf("direct open wrote settings %d times", store.saves)
	}
}

func Test_Model_OpenIDE_UnsetOpensPickerWithoutWrite(t *testing.T) {
	store := &memoryIDEStore{}
	m := NewModel([]string{"api"}, nil)
	m.projectPath = "/work"
	m.servicePaths = map[string]string{"api": "/work/api"}
	m.settings = store
	_, cmd := m.handleGlobalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	if cmd != nil || !m.openWith.open || m.openWith.mode != openWithIDE {
		t.Fatalf("unset IDE did not open selection")
	}
	if store.saves != 0 {
		t.Fatalf("opening selection wrote settings %d times", store.saves)
	}
}

func Test_FileManagerCommand(t *testing.T) {
	tests := []struct {
		goos, name string
		args       []string
	}{
		{"darwin", "open", []string{"/work/app"}},
		{"linux", "xdg-open", []string{"/work/app"}},
		{"windows", "explorer.exe", []string{"/work/app"}},
	}
	for _, tt := range tests {
		name, args, err := fileManagerCommand(tt.goos, "/work/app")
		if err != nil || name != tt.name || len(args) != 1 || args[0] != tt.args[0] {
			t.Fatalf("fileManagerCommand(%q) = %q, %v, %v", tt.goos, name, args, err)
		}
	}
}

func Test_ResolveIDECommand_RejectsWindowsBatchFiles(t *testing.T) {
	file := filepath.Join(t.TempDir(), "code.cmd")
	if err := os.WriteFile(file, []byte("echo code"), 0o700); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	_, err := resolveIDECommand("windows", "code", func(string) (string, error) { return file, nil })
	if !errors.Is(err, errIDECommandUnsupported) {
		t.Fatalf("resolveIDECommand() error = %v, want unsupported", err)
	}
}

func Test_IDECommandNames_Platforms(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want []string
	}{
		{
			name: "macOS",
			goos: "darwin",
			want: []string{"code", "cursor", "zed", "goland", "idea", "webstorm", "pycharm"},
		},
		{
			name: "Linux",
			goos: "linux",
			want: []string{
				"code", "cursor", "zed", "zeditor",
				"goland", "goland.sh",
				"idea", "idea.sh",
				"webstorm", "webstorm.sh",
				"pycharm", "pycharm.sh", "pycharm-professional", "pycharm-community",
			},
		},
		{
			name: "Windows",
			goos: "windows",
			want: []string{
				"code.exe", "Cursor.exe", "zed.exe",
				"goland64.exe", "goland.exe",
				"idea64.exe", "idea.exe",
				"webstorm64.exe", "webstorm.exe",
				"pycharm64.exe", "pycharm.exe",
			},
		},
		{
			name: "other Unix",
			goos: "freebsd",
			want: []string{"code", "cursor", "zed", "goland", "idea", "webstorm", "pycharm"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ideCommandNames(tt.goos)
			if len(got) != len(tt.want) {
				t.Fatalf("ideCommandNames(%q) = %q, want %q", tt.goos, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("ideCommandNames(%q)[%d] = %q, want %q", tt.goos, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func Test_DetectedIDECommands_FiltersPATHAndStoresLauncherNames(t *testing.T) {
	tests := []struct {
		name  string
		goos  string
		paths map[string]string
		want  []string
	}{
		{
			name:  "Unix keeps found launchers",
			goos:  "linux",
			paths: map[string]string{"code": "/usr/bin/code", "idea.sh": "/opt/idea/bin/idea.sh"},
			want:  []string{"code", "idea.sh"},
		},
		{
			name: "Windows keeps executable files",
			goos: "windows",
			paths: map[string]string{
				"code.exe":    `C:\\Apps\\Code.exe`,
				"Cursor.exe":  `C:\\Apps\\cursor.cmd`,
				"zed.exe":     `C:\\Apps\\zed.com`,
				"goland.exe":  `C:\\Apps\\goland.bat`,
				"pycharm.exe": `C:\\Apps\\pycharm`,
			},
			want: []string{"code.exe", "zed.exe"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectedIDECommands(tt.goos, func(name string) (string, error) {
				path, ok := tt.paths[name]
				if !ok {
					return "", errors.New("not found")
				}
				return path, nil
			})
			if len(got) != len(tt.want) {
				t.Fatalf("detectedIDECommands(%q) = %q, want %q", tt.goos, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("detectedIDECommands(%q)[%d] = %q, want %q", tt.goos, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func Test_Model_OpenWith_SaveServicePreference(t *testing.T) {
	opener := &recordingBrowser{}
	store := &memoryIDEStore{}
	m := NewModel([]string{"api"}, nil)
	m.projectPath = "/work"
	m.servicePaths = map[string]string{"api": "/work/api"}
	m.opener = opener
	m.settings = store
	m.openWith = openWithState{
		open:       true,
		mode:       openWithIDE,
		scope:      scopeService,
		candidates: []string{"goland"},
	}

	_, cmd := m.handleOpenWithKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || store.saves != 1 {
		t.Fatalf("saving service preference returned command %v and %d saves", cmd != nil, store.saves)
	}
	command, source := store.prefs.resolve("/work", "api")
	if command != "goland" || source != "service" {
		t.Fatalf("saved preference = %q, %q, want goland, service", command, source)
	}
	msg := cmd().(externalOpenedMsg)
	if msg.err != nil || opener.command != "goland" || opener.path != "/work/api" {
		t.Fatalf("opened %q with %q, error %v", opener.path, opener.command, msg.err)
	}
}

func Test_Model_Target_AllAndFocusedContainer(t *testing.T) {
	m := NewModel([]string{"web", "docker"}, nil)
	m.projectPath = "/work"
	m.servicePaths = map[string]string{"web": "/work/web", "docker": "/work/infra"}
	path, service := m.target()
	if path != "/work" || service != "" {
		t.Fatalf("all target = %q, %q, want /work and no service", path, service)
	}
	m.active = 2
	m.childFocus["docker"] = "db"
	path, service = m.target()
	if path != "/work/infra" || service != "docker" {
		t.Fatalf("container target = %q, %q, want /work/infra and docker", path, service)
	}
}
