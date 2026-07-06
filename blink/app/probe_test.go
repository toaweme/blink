package app

import (
	"slices"
	"testing"
)

func Test_StableListenPorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []int
		want  []int
	}{
		{
			name:  "nil stays nil",
			ports: nil,
			want:  []int{},
		},
		{
			name:  "keeps registered ports",
			ports: []int{3000, 8080},
			want:  []int{3000, 8080},
		},
		{
			name:  "drops ephemeral tooling port",
			ports: []int{3000, 4206, 56260},
			want:  []int{3000, 4206},
		},
		{
			name:  "boundary: 49151 kept, 49152 dropped",
			ports: []int{49151, 49152},
			want:  []int{49151},
		},
		{
			name:  "all ephemeral falls back to input",
			ports: []int{56260, 61000},
			want:  []int{56260, 61000},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stableListenPorts(tt.ports)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("stableListenPorts(%v) = %v, want %v", tt.ports, got, tt.want)
			}
		})
	}
}
