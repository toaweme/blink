package golang

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// readWorkUses parses a go.work file and returns the paths declared under `use (...)` or `use <path>`. Paths are returned as written; the caller resolves them against the workfile directory.
func readWorkUses(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open go.work: %w", err)
	}
	defer f.Close()

	var (
		uses    []string
		inBlock bool
	)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}
		switch {
		case inBlock:
			if line == ")" {
				inBlock = false
				continue
			}
			uses = append(uses, line)
		case strings.HasPrefix(line, "use "):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "use"))
			if rest == "(" {
				inBlock = true
				continue
			}
			if after, ok := strings.CutPrefix(rest, "("); ok {
				inBlock = true
				rest = strings.TrimSpace(after)
				if rest != "" && rest != ")" {
					uses = append(uses, rest)
				}
				continue
			}
			uses = append(uses, rest)
		case line == "use(":
			inBlock = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read go.work: %w", err)
	}
	return uses, nil
}

func stripComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}
