package erratt

import (
	"errors"
	"fmt"
	"log/slog"
)

// Error is an enhanced error interface that supports:
//   - wrapping other errors
//   - attaching arbitrary key-value attributes
//   - retrieving those attributes for structured logging or inspection
type Error interface {
	error
	With(args ...any) Error // returns a new Error with the supplied attributes appended
	Attrs() []any           // returns all attached attributes (key-value pairs) of the Error and all wrapped errors
	Unwrap() error          // returns the wrapped error, if any
}

type errorWithAttrs struct {
	text       string
	wrappedErr error
	attrs      []any
}

// Ensure *errorWithAttrs implements the standard error interface.
var _ error = &errorWithAttrs{}

// Ensure *errorWithAttrs implements our custom Error interface.
var _ Error = &errorWithAttrs{}

// New constructs an value satisfying the [Error] interface with the
// supplied message and optional key-value attributes. Attributes must
// be supplied in pairs:
//
//	New("something failed", "user_id", 123, "retry", true)
//
// Each call returns a distinct error, even when the text matches.
func New(text string, attrs ...any) Error {
	return &errorWithAttrs{
		text:       text,
		wrappedErr: nil,
		attrs:      attrs,
	}
}

func (ea *errorWithAttrs) Error() string {
	return ea.text
}

func (ea *errorWithAttrs) Attrs() []any {
	attrs := make([]any, len(ea.attrs))
	copy(attrs, ea.attrs)
	wrapped := ea.wrappedErr
	for wrapped != nil {
		if wErr, ok := wrapped.(interface {
			Attrs() []any
		}); ok {
			attrs = append(attrs, wErr.Attrs()...) // nozero
		}
		if wErr, ok := wrapped.(interface {
			Unwrap() error
		}); ok {
			wrapped = wErr.Unwrap()
		} else {
			wrapped = nil
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func (ea *errorWithAttrs) Unwrap() error {
	return ea.wrappedErr
}

func (ea *errorWithAttrs) With(args ...any) Error {
	return &errorWithAttrs{
		text:       ea.text,
		wrappedErr: ea.wrappedErr,
		attrs:      append(ea.attrs, args...),
	}
}

// Errorf creates a formatted Error.
//
//   - A single %w verb wraps the provided error, preserving its attributes.
//   - %w operands must implement error; otherwise %w behaves like %v.
func Errorf(format string, a ...any) Error {
	err := fmt.Errorf(format, a...)
	var wrappedErr error
	// var attrs []any
	if unwerr, ok := err.(interface {
		Unwrap() error
	}); ok {
		wrappedErr = unwerr.Unwrap()
		// if attrErr, ok := wrappedErr.(interface {
		// 	Attrs() []any
		// }); ok {
		// 	attrs = attrErr.Attrs()
		// }
	}
	return &errorWithAttrs{
		text:       err.Error(),
		wrappedErr: wrappedErr,
	}
}

// SlogWarnWith logs err at [slog.LevelWarn] using logger.  If err
// implements the Error interface, its attributes are emitted as
// structured fields.
func SlogWarnWith(err error, logger *slog.Logger) {
	ewa := &errorWithAttrs{}
	if errors.As(err, &ewa) {
		logger.Warn(ewa.text, ewa.Attrs()...)
	} else {
		logger.Warn(err.Error())
	}

}

// SlogWith logs err at [slog.LevelError] using logger.  If err
// implements the Error interface, its attributes are emitted as
// structured fields.
func SlogWith(err error, logger *slog.Logger) {
	ewa := &errorWithAttrs{}
	if errors.As(err, &ewa) {
		logger.Error(ewa.text, ewa.Attrs()...)
	} else {
		logger.Error(err.Error())
	}
}

// SlogWarn logs err at [slog.LevelWarn] using the [slog.Default]
// logger.  If err implements the Error interface, its attributes are
// emitted as structured fields.
func SlogWarn(err error) {
	SlogWarnWith(err, slog.Default())
}

// Slog logs err at [slog.LevelError] using the default logger.  If
// err implements the [Error] interface, its attributes are emitted as
// structured fields.
func Slog(err error) {
	SlogWith(err, slog.Default())
}
