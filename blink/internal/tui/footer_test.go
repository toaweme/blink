package tui

import "testing"

func Test_FormatPortsURL(t *testing.T) {
	tests := []struct {
		name  string
		ports []int
		want  string
	}{
		{"none", nil, ""},
		{"empty", []int{}, ""},
		{"single", []int{8080}, "http://127.0.0.1:8080"},
		{"multiple", []int{8080, 8081, 9090}, "http://127.0.0.1:8080, 8081, 9090"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPortsURL(tt.ports); got != tt.want {
				t.Fatalf("formatPortsURL(%v) = %q, want %q", tt.ports, got, tt.want)
			}
		})
	}
}

func Test_StripTabPrefix(t *testing.T) {
	tests := []struct {
		name, in, tab, want string
	}{
		{"hyphen", "web-api", "web", "api"},
		{"underscore", "web_api", "web", "api"},
		{"no prefix", "api", "web", "api"},
		{"equal to tab", "web", "web", "web"},
		{"prefix only no remainder", "web-", "web", "web-"},
		{"unrelated prefix", "database", "db", "database"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripTabPrefix(tt.in, tt.tab); got != tt.want {
				t.Fatalf("stripTabPrefix(%q, %q) = %q, want %q", tt.in, tt.tab, got, tt.want)
			}
		})
	}
}

func Test_FitLabel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		w    int
		want string
	}{
		{"pads short", "api", 6, "api   "},
		{"exact width", "apiserver", 9, "apiserver"},
		{"truncates with ellipsis", "averylongcontainer", 8, "averylo…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fitLabel(tt.in, tt.w)
			if got != tt.want {
				t.Fatalf("fitLabel(%q, %d) = %q, want %q", tt.in, tt.w, got, tt.want)
			}
			if len([]rune(got)) != tt.w {
				t.Fatalf("fitLabel(%q, %d) width = %d, want %d", tt.in, tt.w, len([]rune(got)), tt.w)
			}
		})
	}
}

func Test_HandleStatusMsg_RecordsPorts(t *testing.T) {
	m := NewModel([]string{"docker"}, nil)

	// service-level (merged) ports keyed by the bare service name.
	next, _ := m.handleStatusMsg(StatusMsg{Service: "docker", Status: "running", Ports: []int{5432, 6379}})
	m = next.(*Model)
	if got := m.ports["docker"]; len(got) != 2 || got[0] != 5432 || got[1] != 6379 {
		t.Fatalf("service ports = %v, want [5432 6379]", got)
	}

	// per-container ports keyed by the service+container composite, so a focused
	// container shows only its own address.
	next, _ = m.handleStatusMsg(StatusMsg{Service: "docker", Child: "db", Status: "running", Ports: []int{5432}})
	m = next.(*Model)
	if got := m.ports["docker"+childSep+"db"]; len(got) != 1 || got[0] != 5432 {
		t.Fatalf("db container ports = %v, want [5432]", got)
	}
}

func Test_RenderRightFooter_PortsFollowFocus(t *testing.T) {
	m := NewModel([]string{"docker"}, nil)
	m.active = 1 // docker tab
	m.ports = map[string][]int{
		"docker":                      {5432, 6379},
		"docker" + childSep + "db":    {5432},
		"docker" + childSep + "redis": {6379},
	}

	// merged view: aggregate ports.
	if got := m.ports[m.viewKey()]; len(got) != 2 {
		t.Fatalf("merged viewKey ports = %v, want 2", got)
	}
	// focus a container: only its port.
	m.childFocus["docker"] = "redis"
	got := m.ports[m.viewKey()]
	if len(got) != 1 || got[0] != 6379 {
		t.Fatalf("focused redis ports = %v, want [6379]", got)
	}
}
