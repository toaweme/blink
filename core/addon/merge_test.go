package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/toaweme/blink/core/config"
)

func TestMergeService(t *testing.T) {
	t.Run("user dir wins over overlay", func(t *testing.T) {
		got := MergeService(
			config.Service{Name: "s", Dir: "user"},
			config.Service{Dir: "overlay"},
		)
		assert.Equal(t, "user", got.Dir)
	})

	t.Run("overlay dir applied when base empty", func(t *testing.T) {
		got := MergeService(
			config.Service{Name: "s"},
			config.Service{Dir: "overlay"},
		)
		assert.Equal(t, "overlay", got.Dir)
	})

	t.Run("commands recurse field by field", func(t *testing.T) {
		got := MergeService(
			config.Service{Commands: config.Commands{
				Run: &config.Command{Command: "./user-bin"},
			}},
			config.Service{Commands: config.Commands{
				Run:   &config.Command{Command: "./default-bin", Service: true},
				Build: &config.Command{Command: "go build"},
			}},
		)
		assert.Equal(t, "./user-bin", got.Commands.Run.Command, "user command wins")
		assert.True(t, got.Commands.Run.Service, "overlay Service flag fills in")
		assert.Equal(t, "go build", got.Commands.Build.Command, "overlay Build used when base has none")
	})

	t.Run("setup commands: overlay runtime install runs before user setup", func(t *testing.T) {
		got := MergeService(
			config.Service{Commands: config.Commands{
				Setup: []config.Command{{Command: "user prep"}},
			}},
			config.Service{Commands: config.Commands{
				Setup: []config.Command{{Command: "npm install"}},
			}},
		)
		var cmds []string
		for _, c := range got.Commands.Setup {
			cmds = append(cmds, c.Command)
		}
		assert.Equal(t, []string{"npm install", "user prep"}, cmds)
	})

	t.Run("slices append overlay then base, deduped", func(t *testing.T) {
		got := MergeService(
			config.Service{Fs: config.Fs{
				Extensions: []string{"yaml", "go"},
				Include:    []string{"../schema"},
			}},
			config.Service{Fs: config.Fs{
				Extensions: []string{"go", "mod", "sum"},
				Include:    []string{"./internal"},
			}},
		)
		assert.Equal(t, []string{"go", "mod", "sum", "yaml"}, got.Fs.Extensions)
		assert.Equal(t, []string{"./internal", "../schema"}, got.Fs.Include)
	})

	t.Run("env: base wins on conflict, overlay fills gaps", func(t *testing.T) {
		got := MergeService(
			config.Service{Env: map[string]string{"BUILD": "user", "EXTRA": "x"}},
			config.Service{Env: map[string]string{"BUILD": "overlay", "DEFAULT": "d"}},
		)
		assert.Equal(t, "user", got.Env["BUILD"])
		assert.Equal(t, "x", got.Env["EXTRA"])
		assert.Equal(t, "d", got.Env["DEFAULT"])
	})

	t.Run("reload booleans OR; service deps append", func(t *testing.T) {
		got := MergeService(
			config.Service{Reload: config.Reload{
				ReloadOnService: []string{"api.schema"},
			}},
			config.Service{Reload: config.Reload{
				Reload:          true,
				ReloadOnService: []string{"docker"},
			}},
		)
		assert.True(t, got.Reload.Reload)
		assert.Equal(t, []string{"docker", "api.schema"}, got.Reload.ReloadOnService)
	})

	t.Run("nil command on one side keeps the other", func(t *testing.T) {
		got := MergeService(
			config.Service{},
			config.Service{Commands: config.Commands{
				Build: &config.Command{Command: "go build"},
			}},
		)
		assert.Equal(t, "go build", got.Commands.Build.Command)
	})

	t.Run("logging level: base wins, empty falls through", func(t *testing.T) {
		got := MergeService(
			config.Service{},
			config.Service{Logging: config.Logging{Level: "debug"}},
		)
		assert.Equal(t, "debug", got.Logging.Level)

		got = MergeService(
			config.Service{Logging: config.Logging{Level: "warn"}},
			config.Service{Logging: config.Logging{Level: "debug"}},
		)
		assert.Equal(t, "warn", got.Logging.Level)
	})
}
