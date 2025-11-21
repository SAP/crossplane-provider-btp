package widget

import (
	"context"
	"fmt"

	"github.com/charmbracelet/huh"
)

func makeSelectOption(options []string) []huh.Option[string] {
	selects := make([]huh.Option[string], len(options))
	for i := range options {
		selects[i] = huh.NewOption(options[i], options[i])
	}
	return selects
}

func multiInputdescFuncFn(items int, selected *[]string) func() string {
	return func() string {
		return fmt.Sprintf("%d/%d selected", len(*selected), items)
	}
}

// MultiInput presents an interactive multi-select widget and returns
// the chosen options.
//
// ctx is used to cancel or time-out the prompt.
// title is displayed above the list.
// options supplies the selectable items.
func MultiInput(ctx context.Context, title string, options []string) ([]string, error) {
	selected := []string{}
	multiSelect := huh.NewMultiSelect[string]().
		Options(
			makeSelectOption(options)...,
		).
		Description(multiInputdescFuncFn(len(options), &selected)()).
		DescriptionFunc(multiInputdescFuncFn(len(options), &selected), &selected).
		Title(title).
		Value(&selected)
	g := huh.NewGroup(multiSelect).WithShowHelp(true)
	f := huh.NewForm(g).WithTheme(huh.ThemeCatppuccin()).WithHeight(10)
	if err := f.RunWithContext(ctx); err != nil {
		return nil, err
	}
	return selected, nil
}
