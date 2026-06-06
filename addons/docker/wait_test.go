package docker

import (
	"reflect"
	"testing"
)

func Test_CollectPorts(t *testing.T) {
	rows := []composePsRow{
		{Service: "web", Publishers: []composePublisher{
			{PublishedPort: 8080, Protocol: "tcp"},
			{PublishedPort: 0}, // unpublished, skipped
		}},
		{Service: "db", Publishers: []composePublisher{
			{PublishedPort: 5432, Protocol: "tcp"},
			{PublishedPort: 5432, Protocol: "tcp"}, // dup, skipped
			{PublishedPort: 53, Protocol: "udp"},   // non-tcp, skipped
		}},
	}

	tests := []struct {
		name   string
		filter []string
		want   []int
	}{
		{"all services", nil, []int{8080, 5432}},
		{"filtered to db", []string{"db"}, []int{5432}},
		{"filtered to unknown", []string{"cache"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectPorts(rows, tt.filter)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("collectPorts(filter=%v) = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}
