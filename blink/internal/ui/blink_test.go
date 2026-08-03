package ui

import (
	"reflect"
	"testing"

	"github.com/toaweme/blink/core/config"
)

func Test_ServiceHostnames(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want map[string]string
	}{
		{name: "empty config", cfg: config.Config{}, want: map[string]string{}},
		{
			name: "only configured hostnames",
			cfg: config.Config{Services: []config.Service{
				{Name: "api", Hostname: "api.localhost"},
				{Name: "worker"},
			}},
			want: map[string]string{"api": "api.localhost"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceHostnames(tt.cfg); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("serviceHostnames() = %v, want %v", got, tt.want)
			}
		})
	}
}
