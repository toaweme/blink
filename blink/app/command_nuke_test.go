package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toaweme/blink/core/config"
)

func Test_Nuke(t *testing.T) {
	tests := []struct {
		name        string
		seed        []string // dirs to create under the temp root
		entries     []string // project-scoped PathEntry paths, relative to root; "" stays empty
		userEntries []string // user-scoped PathEntry paths, relative to root
		global      bool
		skipConfirm bool
		confirm     bool
		wantErr     bool
		wantGone    []string // seeded dirs that must be removed afterwards
		wantKept    []string // seeded dirs that must survive
		wantConfirm bool     // confirm callback must be consulted
		wantOut     string   // substring expected in the output
	}{
		{
			name:    "nothing to remove",
			entries: []string{"state"},
			wantOut: "already clean",
		},
		{
			name:        "skip confirm removes",
			seed:        []string{"state", "logs"},
			entries:     []string{"state", "logs"},
			skipConfirm: true,
			wantGone:    []string{"state", "logs"},
			wantOut:     "blink state has been reset",
		},
		{
			name:        "confirm yes removes",
			seed:        []string{"state"},
			entries:     []string{"state"},
			confirm:     true,
			wantGone:    []string{"state"},
			wantConfirm: true,
			wantOut:     "removed",
		},
		{
			name:        "confirm no aborts",
			seed:        []string{"state"},
			entries:     []string{"state"},
			confirm:     false,
			wantKept:    []string{"state"},
			wantConfirm: true,
			wantOut:     "aborted",
		},
		{
			name:        "empty paths are ignored",
			entries:     []string{""},
			skipConfirm: true,
			wantOut:     "already clean",
		},
		{
			name:        "user-scoped kept without global",
			seed:        []string{"state", "home"},
			entries:     []string{"state"},
			userEntries: []string{"home"},
			skipConfirm: true,
			wantGone:    []string{"state"},
			wantKept:    []string{"home"},
			wantOut:     "keeping user-scoped state",
		},
		{
			name:        "user-scoped removed with global",
			seed:        []string{"state", "home"},
			entries:     []string{"state"},
			userEntries: []string{"home"},
			global:      true,
			skipConfirm: true,
			wantGone:    []string{"state", "home"},
			wantOut:     "blink state has been reset",
		},
		{
			name:        "only user-scoped present without global is a no-op",
			seed:        []string{"home"},
			userEntries: []string{"home"},
			skipConfirm: true,
			wantKept:    []string{"home"},
			wantOut:     "already clean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range tt.seed {
				if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
					t.Fatalf("seed %s: %v", d, err)
				}
			}

			var entries []config.PathEntry
			for _, p := range tt.entries {
				abs := ""
				if p != "" {
					abs = filepath.Join(root, p)
				}
				entries = append(entries, config.PathEntry{Path: abs, Description: p})
			}
			for _, p := range tt.userEntries {
				entries = append(entries, config.PathEntry{Path: filepath.Join(root, p), Description: p, UserScoped: true})
			}

			var out bytes.Buffer
			confirmCalled := false
			confirm := func() bool {
				confirmCalled = true
				return tt.confirm
			}

			err := nuke(entries, tt.global, tt.skipConfirm, confirm, &out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("nuke() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got := out.String(); !strings.Contains(got, tt.wantOut) {
				t.Fatalf("output = %q, want substring %q", got, tt.wantOut)
			}

			for _, d := range tt.wantGone {
				if _, statErr := os.Stat(filepath.Join(root, d)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("%s should have been removed", d)
				}
			}
			for _, d := range tt.wantKept {
				if _, statErr := os.Stat(filepath.Join(root, d)); statErr != nil {
					t.Fatalf("%s should have survived: %v", d, statErr)
				}
			}

			if confirmCalled != tt.wantConfirm {
				t.Fatalf("confirm called = %v, want %v", confirmCalled, tt.wantConfirm)
			}
		})
	}
}
