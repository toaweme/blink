// Package configform owns the interactive forms behind zero-config
// `blink run`: the detected-services picker. It reads and writes the
// config.Config struct directly.
package configform

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"

	"github.com/toaweme/blink/core/config"
	"github.com/toaweme/blink/core/config/detect"
)

// PickDetected shows a pre-checked multiselect of detected services and
// returns the ones the user keeps. Every entry starts selected, so hitting
// enter keeps everything. Returns (nil, nil) when the user cancels (esc).
func PickDetected(detected []detect.Detected) ([]config.Service, error) {
	if len(detected) == 0 {
		return nil, nil
	}

	// index-valued options so duplicate labels never collide.
	selected := make([]int, 0, len(detected))
	opts := make([]huh.Option[int], 0, len(detected))
	for i, d := range detected {
		label := d.Label
		if label == "" {
			label = d.Service.Name
		}
		opts = append(opts, huh.NewOption(label, i).Selected(true))
		selected = append(selected, i)
	}

	if err := Run(huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[int]().
			Title("Detected services").
			Description("Space toggles, enter confirms. Everything is kept by default.").
			Options(opts...).
			Value(&selected),
	))); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to run detected-services picker: %w", err)
	}

	out := make([]config.Service, 0, len(selected))
	for _, i := range selected {
		if i >= 0 && i < len(detected) {
			out = append(out, detected[i].Service)
		}
	}
	return out, nil
}
