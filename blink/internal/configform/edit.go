package configform

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/toaweme/blink/core/config"
)

// EditService opens a single-screen editor for one service, dispatched on its
// runtime. Detected services arrive pre-filled. others is the set of sibling
// service names, used to reject a duplicate rename. Examples live in field
// descriptions, not placeholders, which would read like real typed-in values.
func EditService(svc *config.Service, others []string) error {
	switch svc.Runtime {
	case "go":
		return editGo(svc, others)
	case "docker":
		return editDocker(svc, others)
	default:
		return editShell(svc, others)
	}
}

func nameField(svc *config.Service, others []string) *huh.Input {
	return huh.NewInput().
		Title("Name").
		Description("shown in the TUI and referenced by dependents").
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New("required")
			}
			for _, o := range others {
				if o == s {
					return fmt.Errorf("name %q already used", s)
				}
			}
			return nil
		}).
		Value(&svc.Name)
}

func editShell(svc *config.Service, others []string) error {
	run := ""
	if svc.Commands.Run != nil {
		run = svc.Commands.Run.Command
	}
	reload := svc.Reload.Reload
	ports := joinPorts(svc.Ports)

	if err := Run(huh.NewForm(huh.NewGroup(
		nameField(svc, others),
		huh.NewInput().
			Title("Run command").
			Description("the long-running command (e.g. ./bin/api, npm run dev)").
			Value(&run),
		huh.NewConfirm().
			Title("Restart on file change?").
			Affirmative("Yes").Negative("No").
			Value(&reload),
		huh.NewInput().
			Title("Ports").
			Description("comma-separated, killed before start to clear orphans (e.g. 8080, 9090)").
			Value(&ports),
	))); err != nil {
		return abortOrErr(err, "shell")
	}

	if run != "" {
		if svc.Commands.Run == nil {
			svc.Commands.Run = &config.Command{Service: true}
		}
		svc.Commands.Run.Command = run
	}
	svc.Reload.Reload = reload
	svc.Ports = parsePorts(ports)
	return nil
}

func editGo(svc *config.Service, others []string) error {
	if svc.Go == nil {
		svc.Go = &config.GoConfig{}
	}
	args := joinCSV(svc.Go.Args)
	reload := svc.Reload.Reload
	ports := joinPorts(svc.Ports)

	if err := Run(huh.NewForm(huh.NewGroup(
		nameField(svc, others),
		huh.NewInput().
			Title("Go package").
			Description("path passed to `go build` (e.g. ./cmd/api)").
			Validate(notEmpty).
			Value(&svc.Go.Package),
		huh.NewInput().
			Title("Binary args").
			Description("comma-separated, optional (e.g. --port, 8080)").
			Value(&args),
		huh.NewConfirm().
			Title("Restart on file change?").
			Affirmative("Yes").Negative("No").
			Value(&reload),
		huh.NewInput().
			Title("Ports").
			Description("comma-separated, optional (e.g. 8080)").
			Value(&ports),
	))); err != nil {
		return abortOrErr(err, "go")
	}
	svc.Go.Args = parseCSV(args)
	svc.Reload.Reload = reload
	svc.Ports = parsePorts(ports)
	return nil
}

func editDocker(svc *config.Service, others []string) error {
	if svc.Docker == nil {
		svc.Docker = &config.DockerConfig{}
	}
	all := svc.Docker.Services
	stop := svc.Docker.StopOnExit

	// the Confirm comes before the MultiSelect: a MultiSelect owns ↑/↓ for its
	// options, so a field after it is reachable only by tab/enter. Keeping it last
	// leaves every other field arrow-navigable.
	group := []huh.Field{
		nameField(svc, others),
		huh.NewConfirm().
			Title("Stop containers when blink exits?").
			Description("Keep leaves them running so the next start reuses warm databases (recommended)").
			Affirmative("Stop").Negative("Keep").
			Value(&stop),
	}
	var chosen []string
	if len(all) > 0 {
		chosen = append([]string{}, all...)
		opts := make([]huh.Option[string], 0, len(all))
		for _, name := range all {
			opts = append(opts, huh.NewOption(name, name).Selected(true))
		}
		group = append(group, huh.NewMultiSelect[string]().
			Title("Compose services to run").
			Description("unchecked services stay out of the stack").
			Options(opts...).
			Value(&chosen))
	}

	if err := Run(huh.NewForm(huh.NewGroup(group...))); err != nil {
		return abortOrErr(err, "docker")
	}

	// empty selection means "all" in DockerConfig; only narrow when the user
	// actually dropped some.
	if len(all) > 0 && len(chosen) != len(all) {
		svc.Docker.Services = chosen
	}
	svc.Docker.StopOnExit = stop
	return nil
}

// abortOrErr maps a huh esc/ctrl+c into a nil error (edit canceled, keep the
// service as-is) and wraps any real failure with the form name.
func abortOrErr(err error, form string) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	return fmt.Errorf("failed to run %s editor: %w", form, err)
}

func notEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("required")
	}
	return nil
}

func parseCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinCSV(s []string) string { return strings.Join(s, ", ") }

// parsePorts reads the comma-separated ports field: a numeric entry is a literal
// port, anything else is an env-var name. Empty entries are dropped.
func parsePorts(s string) []config.Port {
	var out []config.Port
	for _, tok := range parseCSV(s) {
		p, err := config.ParsePort(tok)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

func joinPorts(ports []config.Port) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, p.String())
	}
	return strings.Join(parts, ", ")
}
