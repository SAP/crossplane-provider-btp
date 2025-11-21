package erratt_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
)

func ExampleNew() {
	err := erratt.New("test error")
	fmt.Print(err.Error())
	//output: test error
}

func ExampleNew_attributes() {
	err := erratt.New("test error", "reason", "test")
	fmt.Print(err.Error())
	fmt.Print(" ")
	fmt.Print(err.Attrs()[0])
	fmt.Print("=")
	fmt.Print(err.Attrs()[1])
	//output: test error reason=test
}

func ExampleErrorf() {
	err := erratt.Errorf("test error")
	fmt.Print(err.Error())
	//output: test error
}

func ExampleErrorf_wrap() {
	err := erratt.Errorf("test error: %w", errors.New("wrapped error"))
	fmt.Print(err.Error())
	//output: test error: wrapped error
}

func ExampleErrorf_attributes() {
	err := erratt.Errorf("test error: %w", errors.New("wrapped error")).With("reason", "test")
	fmt.Print(err.Error())
	fmt.Print(" ")
	fmt.Print(err.Attrs()[0])
	fmt.Print("=")
	fmt.Print(err.Attrs()[1])
	//output: test error: wrapped error reason=test
}

func ExampleSlog() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     nil,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove time from the output for predictable test output.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	err := erratt.New("this is an error")
	erratt.Slog(err)
	//output: level=ERROR msg="this is an error"
}

func ExampleSlog_attributes() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     nil,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove time from the output for predictable test output.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	err := erratt.New("this is an error", "reason", "test")
	erratt.Slog(err)
	//output: level=ERROR msg="this is an error" reason=test
}

func ExampleSlog_wrapped() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     nil,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove time from the output for predictable test output.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	errInternal := errors.New("internal error")
	err := erratt.Errorf("error occurred: %w", errInternal)
	erratt.Slog(err)
	//output: level=ERROR msg="error occurred: internal error"
}

func ExampleSlog_wrapped_with_attributes() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     nil,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove time from the output for predictable test output.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	errInternal := erratt.New("internal error", "reason", "test")
	err := erratt.Errorf("error occurred: %w", errInternal)
	erratt.Slog(err)
	//output: level=ERROR msg="error occurred: internal error" reason=test
}

func ExampleSlog_multilevel_wrap() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     nil,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove time from the output for predictable test output.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	errDeepest := erratt.New("deepest error", "deepest_reason", "failure")
	errMedium := fmt.Errorf("medium error: %w", errDeepest)
	err := erratt.Errorf("outer error: %w", errMedium).With("outer_reason", "error")
	erratt.Slog(err)
	//output: level=ERROR msg="outer error: medium error: deepest error" outer_reason=error deepest_reason=failure
}
