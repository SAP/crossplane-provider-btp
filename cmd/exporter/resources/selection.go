package resources

import (
	"context"
	"fmt"

	"github.com/SAP/xp-clifford/cli/configparam"
)

// Options controls resource selection and export behavior.
type Options struct {
	ResolveReferences bool
	SelectAll         bool
}

// SelectCache applies a resource selector to cache. When SelectAll is enabled,
// the cache is retained unchanged and no interactive selection is requested.
func SelectCache[T BtpResource](ctx context.Context, cache ResourceCache[T], param *configparam.StringSliceParam, options Options) error {
	if options.SelectAll {
		return nil
	}

	widgetValues := cache.ValuesForSelection()
	param.WithPossibleValuesFn(func() ([]string, error) {
		return widgetValues.Values(), nil
	})

	selectedValues, err := param.ValueOrAsk(ctx)
	if err != nil {
		return fmt.Errorf("failed to get parameter value: %s: %w", param.GetName(), err)
	}

	cache.KeepSelectedOnly(selectedValues)
	return nil
}
