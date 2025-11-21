/*
Package erratt provides enhanced Go errors with attributes and wrapping.

An [Error] behaves like a standard error while also supporting:

  - Attributes: arbitrary key-value metadata that integrate seamlessly
    with the [log/slog] package for structured logging.
  - Wrapping: composition of errors, compatible with [errors.Unwrap].

Create errors with [New] or [Errorf], then enrich them with attributes:

	simple := New("connection refused")
	rich := New("auth failed", "user", 42, "status", 403)
	wrapped := Errorf("call: %w", rich).With("retry", false)

Log any error—standard or enhanced—using the package helpers:

	Slog(logger, wrapped)
	SlogWarn(logger, simple)
*/
package erratt
