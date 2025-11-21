package widget

import (
	"context"

	"github.com/charmbracelet/huh"
)

// TextInput presents an interactive single-line prompt and returns the entered text.
//
// ctx is used to cancel or time-out the prompt.
// title is displayed above the input field.
// placeholder is shown when the field is empty.
// sensitive hides keystrokes when true.
func TextInput(ctx context.Context, title, placeholder string, sensitive bool) (string, error) {
	var value string
	echoMode := huh.EchoModeNormal
	if sensitive {
		echoMode = huh.EchoModePassword
	}
	input := huh.NewInput().
		Value(&value).
		Title(title).
		Placeholder(placeholder).
		EchoMode(echoMode)
	g := huh.NewGroup(input).WithShowHelp(false)
	f := huh.NewForm(g).WithTheme(huh.ThemeCatppuccin())
	if err := f.RunWithContext(ctx); err != nil {
		return "", err
	}
	return value, nil
}
