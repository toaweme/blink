package addon

import "github.com/toaweme/blink/core/config"

// MergeService overlays runtime defaults onto a user-authored service.
//
// Rules:
//   - Scalars (strings, bools): user (base) wins if non-zero; overlay otherwise.
//   - Pointer structs (*Command): recurse field-by-field; if one side is nil, take the other.
//   - Slices: append(overlay, base...) so runtime-provided defaults come first and user additions are preserved.
//   - Maps (Env): overlay seeds the map; user keys override on conflict.
//   - Nested structs (Fs, Reload, Commands, Logging): recurse.
//
// Name, Dir and Runtime come from base: the user always names and places their
// own service, even when a runtime is involved.
func MergeService(base, overlay config.Service) config.Service {
	out := base

	if out.Dir == "" {
		out.Dir = overlay.Dir
	}

	out.Commands = mergeCommands(base.Commands, overlay.Commands)
	out.Fs = mergeFs(base.Fs, overlay.Fs)
	out.Reload = mergeReload(base.Reload, overlay.Reload)
	out.Env = mergeEnv(base.Env, overlay.Env)
	out.Logging = mergeLogging(base.Logging, overlay.Logging)
	out.Ports = dedupPorts(append(append([]config.Port{}, overlay.Ports...), base.Ports...))
	if base.ForceShutdown == nil {
		out.ForceShutdown = overlay.ForceShutdown
	}

	return out
}

// dedupPorts drops empty and duplicate entries, keying on the canonical string
// form so a literal and its env reference (e.g. 8080 and PORT) are kept as
// the distinct entries they are while exact repeats collapse.
func dedupPorts(in []config.Port) []config.Port {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]config.Port, 0, len(in))
	for _, p := range in {
		if p.EnvKey == "" && p.Value <= 0 {
			continue
		}
		key := p.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

func mergeCommands(base, overlay config.Commands) config.Commands {
	return config.Commands{
		// runtime-contributed setup (overlay) runs before any the user added
		// (base), so dependency installs precede custom preparation.
		Setup: append(append([]config.Command{}, overlay.Setup...), base.Setup...),
		Build: mergeCommandPtr(base.Build, overlay.Build),
		Run:   mergeCommandPtr(base.Run, overlay.Run),
	}
}

func mergeCommandPtr(base, overlay *config.Command) *config.Command {
	switch {
	case base == nil && overlay == nil:
		return nil
	case base == nil:
		c := *overlay
		return &c
	case overlay == nil:
		return base
	}
	merged := *base
	if merged.Command == "" {
		merged.Command = overlay.Command
	}
	if merged.CommandCleanup == "" {
		merged.CommandCleanup = overlay.CommandCleanup
	}
	if merged.Dir == "" {
		merged.Dir = overlay.Dir
	}
	if !merged.Service {
		merged.Service = overlay.Service
	}
	merged.Before = append(append([]config.Command{}, overlay.Before...), base.Before...)
	merged.After = append(append([]config.Command{}, overlay.After...), base.After...)
	return &merged
}

func mergeFs(base, overlay config.Fs) config.Fs {
	return config.Fs{
		Extensions: dedupStrings(append(append([]string{}, overlay.Extensions...), base.Extensions...)),
		Include:    dedupStrings(append(append([]string{}, overlay.Include...), base.Include...)),
		Exclude:    dedupStrings(append(append([]string{}, overlay.Exclude...), base.Exclude...)),
	}
}

func mergeReload(base, overlay config.Reload) config.Reload {
	out := base
	if !out.Reload {
		out.Reload = overlay.Reload
	}
	out.ReloadOnDelete = dedupStrings(append(append([]string{}, overlay.ReloadOnDelete...), base.ReloadOnDelete...))
	out.ReloadOnService = dedupStrings(append(append([]string{}, overlay.ReloadOnService...), base.ReloadOnService...))
	return out
}

func mergeEnv(base, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(overlay))
	for k, v := range overlay {
		out[k] = v
	}
	for k, v := range base {
		out[k] = v
	}
	return out
}

func mergeLogging(base, overlay config.Logging) config.Logging {
	out := base
	if out.Level == "" {
		out.Level = overlay.Level
	}
	return out
}

func dedupStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
