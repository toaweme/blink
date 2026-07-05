package tui

import "testing"

// Test_NewModel_AllTabForMultipleServices asserts the aggregate "all" tab is
// only present with more than one service. A lone service opens straight on
// itself (no all-tab, which would just duplicate its logs without the
// per-service verbs), while multiple services get the all-tab at index 0.
func Test_NewModel_AllTabForMultipleServices(t *testing.T) {
	tests := []struct {
		name     string
		services []string
		wantTabs []string
	}{
		{"lone service has no all-tab", []string{"web"}, []string{"web"}},
		{"two services get the all-tab", []string{"web", "api"}, []string{allTab, "web", "api"}},
		{"zero services keep the all-tab", nil, []string{allTab}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(tt.services, nil)
			if !equalStrings(m.tabs, tt.wantTabs) {
				t.Fatalf("tabs = %v, want %v", m.tabs, tt.wantTabs)
			}
			// the opening tab must always be a valid index.
			if got := m.activeTab(); got != tt.wantTabs[0] {
				t.Fatalf("activeTab = %q, want %q", got, tt.wantTabs[0])
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
