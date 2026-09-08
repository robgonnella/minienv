// Package errs is the error mechanism shared by every internal package. The
// kinds themselves stay in the package that raises them; only the machinery
// lives here.
package errs

import (
	"errors"
	"fmt"
)

// Kind classifies a failure. It implements error so it doubles as the sentinel
// callers match on: errors.Is(err, config.ErrInvalidPort). Kind values are
// namespaced by package ("config.invalid_port") because they share one type,
// so the compiler cannot stop a loader kind being compared to a config error.
//
//nolint:errname // Kind is a classification that implements error, not an error
type Kind string

func (k Kind) Error() string { return string(k) }

// Error carries a Kind alongside the message, so a failure branch can be
// identified without matching on prose.
type Error struct {
	kind  Kind
	msg   string
	cause error
}

// Errorf builds an Error of the given kind. A %w verb in format captures the
// wrapped cause, so errors.Is/As traverse into it.
func Errorf(kind Kind, format string, args ...any) *Error {
	//nolint:err113 // this is the wrapper the rule exists to push callers to
	wrapped := fmt.Errorf(format, args...)
	return &Error{kind: kind, msg: wrapped.Error(), cause: errors.Unwrap(wrapped)}
}

func (e *Error) Kind() Kind    { return e.kind }
func (e *Error) Error() string { return e.msg }
func (e *Error) Unwrap() error { return e.cause }

// Is reports whether target is this error's Kind, which is what lets
// errors.Is match a kind. Walking Unwrap chains and errors.Join trees is
// errors.Is's job, not ours.
func (e *Error) Is(target error) bool { return target == e.kind }
