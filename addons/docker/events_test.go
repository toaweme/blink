package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseComposePs_Array(t *testing.T) {
	data := []byte(`[
  {"Name":"awee-db-1","Service":"db","State":"running","Health":"healthy"},
  {"Name":"awee-redis-1","Service":"redis","State":"running","Health":""}
]`)
	rows, err := parseComposePs(data)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "db", rows[0].Service)
	assert.Equal(t, "running", rows[0].State)
}

func Test_ParseComposePs_NDJSON(t *testing.T) {
	data := []byte(`{"Name":"awee-db-1","Service":"db","State":"running","Health":"healthy"}
{"Name":"awee-redis-1","Service":"redis","State":"exited","Health":""}
`)
	rows, err := parseComposePs(data)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "redis", rows[1].Service)
	assert.Equal(t, "exited", rows[1].State)
}

func Test_MapEventAction(t *testing.T) {
	cases := map[string]string{
		"start":                    "running",
		"die":                      "exited",
		"kill":                     "exited",
		"stop":                     "exited",
		"health_status: healthy":   "running",
		"health_status: unhealthy": "crashed",
		"create":                   "building",
		"destroy":                  "stopped",
		"exec_start":               "",
		"top":                      "",
	}
	for input, want := range cases {
		assert.Equal(t, want, mapEventAction(input), input)
	}
}

func Test_NormaliseState(t *testing.T) {
	assert.Equal(t, "running", normaliseState("running", "healthy"))
	assert.Equal(t, "crashed", normaliseState("running", "unhealthy"))
	assert.Equal(t, "exited", normaliseState("exited", ""))
	assert.Equal(t, "restarting", normaliseState("restarting", ""))
}
