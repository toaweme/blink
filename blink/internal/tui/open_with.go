package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	clicfg "github.com/toaweme/cli/config"

	"github.com/toaweme/blink/blink/internal/theme"
	"github.com/toaweme/blink/core/config/format"
)

var errIDECommandUnsupported = errors.New("IDE command is unsupported")

type idePreferences struct {
	IDE idePreferenceSet `yaml:"ide,omitempty"`
}

type idePreferenceSet struct {
	Default  string                 `yaml:"default,omitempty"`
	Projects []projectIDEPreference `yaml:"projects,omitempty"`
}

type projectIDEPreference struct {
	Root       string            `yaml:"root"`
	Executable string            `yaml:"executable,omitempty"`
	Services   map[string]string `yaml:"services,omitempty"`
}

func (p *idePreferences) resolve(root, service string) (string, string) {
	root = filepath.Clean(root)
	for _, project := range p.IDE.Projects {
		if filepath.Clean(project.Root) != root {
			continue
		}
		if service != "" {
			if command := project.Services[service]; command != "" {
				return command, "service"
			}
		}
		if project.Executable != "" {
			return project.Executable, "project"
		}
	}
	if p.IDE.Default != "" {
		return p.IDE.Default, "global"
	}
	return "", ""
}

type preferenceScope int

const (
	scopeGlobal preferenceScope = iota
	scopeProject
	scopeService
)

func (p *idePreferences) set(scope preferenceScope, root, service, command string) error {
	if scope == scopeGlobal {
		p.IDE.Default = command
		return nil
	}
	root = filepath.Clean(root)
	idx := -1
	for i := range p.IDE.Projects {
		if filepath.Clean(p.IDE.Projects[i].Root) == root {
			idx = i
			break
		}
	}
	if idx < 0 {
		p.IDE.Projects = append(p.IDE.Projects, projectIDEPreference{Root: root})
		idx = len(p.IDE.Projects) - 1
	}
	if scope == scopeProject {
		p.IDE.Projects[idx].Executable = command
		return nil
	}
	if service == "" {
		return fmt.Errorf("failed to save service IDE preference: %w", errIDECommandUnsupported)
	}
	if p.IDE.Projects[idx].Services == nil {
		p.IDE.Projects[idx].Services = map[string]string{}
	}
	p.IDE.Projects[idx].Services[service] = command
	return nil
}

type idePreferenceStore interface {
	Load() (idePreferences, error)
	Save(idePreferences) error
}

type configIDEPreferenceStore struct {
	store clicfg.Store
}

var _ idePreferenceStore = (*configIDEPreferenceStore)(nil)

func newYAMLIDEPreferenceStore(configHome string) idePreferenceStore {
	if configHome == "" {
		return errorIDEPreferenceStore{err: fmt.Errorf("failed to configure IDE settings without ConfigHome: %w", errIDECommandUnsupported)}
	}
	codec, err := format.CodecForPath("settings.yml")
	if err != nil {
		return errorIDEPreferenceStore{err: err}
	}
	return &configIDEPreferenceStore{store: clicfg.NewFileStore(configHome, "settings.yml", true, codec)}
}

func (s *configIDEPreferenceStore) Load() (idePreferences, error) {
	var prefs idePreferences
	if err := s.store.Read(&prefs); errors.Is(err, clicfg.ErrConfigNotFound) {
		return idePreferences{}, nil
	} else if err != nil {
		return idePreferences{}, fmt.Errorf("failed to decode IDE settings: %w", err)
	}
	return prefs, nil
}

func (s *configIDEPreferenceStore) Save(prefs idePreferences) error {
	if err := s.store.Write(prefs); err != nil {
		return fmt.Errorf("failed to save IDE settings: %w", err)
	}
	return nil
}

type errorIDEPreferenceStore struct{ err error }

var _ idePreferenceStore = errorIDEPreferenceStore{}

func (s errorIDEPreferenceStore) Load() (idePreferences, error) { return idePreferences{}, s.err }
func (s errorIDEPreferenceStore) Save(idePreferences) error     { return s.err }

func resolveIDECommand(goos, command string, lookPath func(string) (string, error)) (string, error) {
	if command == "" {
		return "", fmt.Errorf("failed to resolve an empty IDE command: %w", errIDECommandUnsupported)
	}
	resolved := command
	if !filepath.IsAbs(command) {
		var err error
		resolved, err = lookPath(command)
		if err != nil {
			return "", fmt.Errorf("failed to find IDE command %q: %w", command, err)
		}
	}
	ext := strings.ToLower(filepath.Ext(resolved))
	if goos == "windows" && (ext == ".cmd" || ext == ".bat") {
		return "", fmt.Errorf("failed to use Windows IDE launcher %q: %w", resolved, errIDECommandUnsupported)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to inspect IDE command %q: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("failed to use non-file IDE command %q: %w", resolved, errIDECommandUnsupported)
	}
	return resolved, nil
}

type openWithMode int

const (
	openWithActions openWithMode = iota
	openWithIDE
)

type openWithState struct {
	open       bool
	mode       openWithMode
	cursor     int
	scope      preferenceScope
	candidates []string
	prefs      idePreferences
}

type externalOpenedMsg struct{ err error }

func (m *Model) target() (string, string) {
	service := m.activeTab()
	if service == allTab {
		return m.projectPath, ""
	}
	path := m.servicePaths[service]
	if path == "" {
		path = m.projectPath
	}
	return path, service
}

func (m *Model) openIDE() tea.Cmd {
	prefs, err := m.settings.Load()
	if err != nil {
		m.setFlash("SETTINGS FAILED", theme.Danger)
		return nil
	}
	_, service := m.target()
	command, _ := prefs.resolve(m.projectPath, service)
	if command == "" {
		m.openOpenWith(true)
		return nil
	}
	path, _ := m.target()
	return func() tea.Msg { return externalOpenedMsg{err: m.opener.OpenIDE(command, path)} }
}

func (m *Model) openOpenWith(selectIDE bool) {
	prefs, err := m.settings.Load()
	if err != nil {
		m.setFlash("SETTINGS FAILED", theme.Danger)
		return
	}
	m.openWith = openWithState{open: true, prefs: prefs, scope: scopeProject}
	if selectIDE {
		m.openIDEPicker()
	}
}

func (m *Model) openIDEPicker() {
	m.openWith.mode = openWithIDE
	m.openWith.cursor = 0
	m.openWith.candidates = detectedIDECommands(runtime.GOOS, exec.LookPath)
	if m.openWith.prefs.IDE.Default == "" {
		m.openWith.scope = scopeGlobal
	}
}

func detectedIDECommands(goos string, lookPath func(string) (string, error)) []string {
	var out []string
	for _, name := range ideCommandNames(goos) {
		path, err := lookPath(name)
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		if goos == "windows" && ext != ".exe" && ext != ".com" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func ideCommandNames(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"code", "cursor", "zed", "goland", "idea", "webstorm", "pycharm"}
	case "linux":
		return []string{
			"code", "cursor", "zed", "zeditor",
			"goland", "goland.sh",
			"idea", "idea.sh",
			"webstorm", "webstorm.sh",
			"pycharm", "pycharm.sh", "pycharm-professional", "pycharm-community",
		}
	case "windows":
		return []string{
			"code.exe", "Cursor.exe", "zed.exe",
			"goland64.exe", "goland.exe",
			"idea64.exe", "idea.exe",
			"webstorm64.exe", "webstorm.exe",
			"pycharm64.exe", "pycharm.exe",
		}
	default:
		return []string{"code", "cursor", "zed", "goland", "idea", "webstorm", "pycharm"}
	}
}

func (m *Model) handleOpenWithKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.openWith = openWithState{}
		return m, nil
	}
	if m.openWith.mode == openWithActions {
		return m.handleOpenWithActionKey(msg)
	}
	return m.handleIDEPickerKey(msg)
}

func (m *Model) handleOpenWithActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.openWith.cursor > 0 {
			m.openWith.cursor--
		}
	case "down", "j":
		if m.openWith.cursor < 2 {
			m.openWith.cursor++
		}
	case "enter":
		path, service := m.target()
		switch m.openWith.cursor {
		case 0:
			m.openWith = openWithState{}
			return m, func() tea.Msg { return externalOpenedMsg{err: m.opener.OpenFileManager(path)} }
		case 1:
			command, _ := m.openWith.prefs.resolve(m.projectPath, service)
			if command == "" {
				m.openIDEPicker()
				return m, nil
			}
			m.openWith = openWithState{}
			return m, func() tea.Msg { return externalOpenedMsg{err: m.opener.OpenIDE(command, path)} }
		case 2:
			m.openIDEPicker()
		}
	}
	return m, nil
}

func (m *Model) handleIDEPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.openWith.cursor > 0 {
			m.openWith.cursor--
		}
	case "down", "j":
		if m.openWith.cursor < len(m.openWith.candidates)-1 {
			m.openWith.cursor++
		}
	case "left", "h":
		if m.openWith.scope > scopeGlobal {
			m.openWith.scope--
		}
	case "right", "l":
		maxScope := scopeService
		if _, service := m.target(); service == "" {
			maxScope = scopeProject
		}
		if m.openWith.scope < maxScope {
			m.openWith.scope++
		}
	case "enter":
		if len(m.openWith.candidates) == 0 {
			return m, nil
		}
		command := m.openWith.candidates[m.openWith.cursor]
		_, service := m.target()
		if err := m.openWith.prefs.set(m.openWith.scope, m.projectPath, service, command); err != nil {
			m.setFlash("SETTINGS FAILED", theme.Danger)
			return m, nil
		}
		if err := m.settings.Save(m.openWith.prefs); err != nil {
			m.setFlash("SETTINGS FAILED", theme.Danger)
			return m, nil
		}
		path, _ := m.target()
		m.openWith = openWithState{}
		return m, func() tea.Msg { return externalOpenedMsg{err: m.opener.OpenIDE(command, path)} }
	}
	return m, nil
}

func (m *Model) renderOpenWithDialog() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	selected := lipgloss.NewStyle().Foreground(theme.Cursor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.Muted)
	lines := []string{title.Render("OPEN WITH"), ""}
	if m.openWith.mode == openWithActions {
		_, service := m.target()
		command, source := m.openWith.prefs.resolve(m.projectPath, service)
		labels := []string{"File manager", "IDE"}
		if command != "" {
			labels[1] += " (" + command + ", " + source + ")"
		}
		labels = append(labels, "Configure IDE")
		for i, label := range labels {
			prefix := "  "
			if i == m.openWith.cursor {
				prefix = selected.Render("› ")
			}
			lines = append(lines, prefix+label)
		}
	} else {
		scopes := []string{"Global", "Project", "Service"}
		lines = append(lines, dim.Render("Save for ")+selected.Render(scopes[m.openWith.scope]), "")
		if len(m.openWith.candidates) == 0 {
			lines = append(lines, dim.Render("No supported IDE launcher found in PATH"))
		}
		for i, command := range m.openWith.candidates {
			prefix := "  "
			if i == m.openWith.cursor {
				prefix = selected.Render("› ")
			}
			lines = append(lines, prefix+command)
		}
	}
	lines = append(lines, "", dim.Render("↑/↓ choose · ←/→ scope · enter confirm · esc close"))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Accent).Padding(1, 3).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
