package forge

import (
	"errors"
	"fmt"
)

// ErrorClass tells a caller what kind of failure it is looking at, so retry
// and rendering policy never has to string-match a provider's message.
type ErrorClass string

const (
	// ClassRetryable means the same call may succeed if repeated later:
	// rate limits, 5xx, timeouts, transient conflicts.
	ClassRetryable ErrorClass = "retryable"
	// ClassPermanent means repeating the call will not help: the repo does not
	// exist, the input was malformed, the response was unparseable.
	ClassPermanent ErrorClass = "permanent"
	// ClassUnavailable means the transport itself is not usable right now:
	// `gh` is not installed or not authenticated, the forge is unreachable, no
	// token was supplied. Callers must degrade to "unknown" — never to
	// "absent", "empty", or "green".
	ClassUnavailable ErrorClass = "unavailable"
	// ClassUnsupported means this provider cannot answer this question at all,
	// no matter the credentials or the network.
	ClassUnsupported ErrorClass = "unsupported"
)

// Sentinels for errors.Is. A *Error matches the sentinel for its own class:
//
//	if errors.Is(err, forge.ErrUnavailable) { renderUnknown() }
var (
	ErrRetryable   = errors.New("forge: retryable")
	ErrPermanent   = errors.New("forge: permanent")
	ErrUnavailable = errors.New("forge: unavailable")
	ErrUnsupported = errors.New("forge: unsupported")
)

func (c ErrorClass) sentinel() error {
	switch c {
	case ClassRetryable:
		return ErrRetryable
	case ClassPermanent:
		return ErrPermanent
	case ClassUnavailable:
		return ErrUnavailable
	case ClassUnsupported:
		return ErrUnsupported
	default:
		return nil
	}
}

// Error is a classified forge failure. Provider is the provider name
// ("github", "forgejo"), Op the operation ("ListPRs"), and Err the underlying
// cause, which is preserved for errors.Is/As.
type Error struct {
	Class    ErrorClass
	Provider string
	Op       string
	Message  string
	Err      error
}

func (e *Error) Error() string {
	prefix := e.Provider
	if prefix == "" {
		prefix = "forge"
	}
	if e.Op != "" {
		prefix += " " + e.Op
	}
	msg := e.Message
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	if msg == "" {
		msg = string(e.Class)
		return fmt.Sprintf("%s: %s", prefix, msg)
	}
	return fmt.Sprintf("%s: %s (%s)", prefix, msg, e.Class)
}

// Unwrap exposes the underlying cause.
func (e *Error) Unwrap() error { return e.Err }

// Is matches the sentinel for this error's class, so callers can write
// errors.Is(err, forge.ErrRetryable) without depending on the concrete type.
func (e *Error) Is(target error) bool {
	if s := e.Class.sentinel(); s != nil && target == s {
		return true
	}
	return false
}

// Errorf builds a classified error. Use it in providers so every error leaving
// a provider carries a class.
func Errorf(class ErrorClass, provider, op string, cause error, format string, args ...any) *Error {
	return &Error{
		Class:    class,
		Provider: provider,
		Op:       op,
		Message:  fmt.Sprintf(format, args...),
		Err:      cause,
	}
}

// ClassOf reports the class of an error. It returns "" for a nil error and
// ClassPermanent for an error that carries no class — failing closed, because
// retrying a failure nobody classified is how a poller becomes a hot loop.
func ClassOf(err error) ErrorClass {
	if err == nil {
		return ""
	}
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Class
	}
	switch {
	case errors.Is(err, ErrRetryable):
		return ClassRetryable
	case errors.Is(err, ErrUnavailable):
		return ClassUnavailable
	case errors.Is(err, ErrUnsupported):
		return ClassUnsupported
	case errors.Is(err, ErrPermanent):
		return ClassPermanent
	}
	return ClassPermanent
}

// IsRetryable reports whether err is worth retrying.
func IsRetryable(err error) bool { return ClassOf(err) == ClassRetryable }

// IsUnavailable reports whether the transport was unusable — the signal to
// render "unknown" rather than a state.
func IsUnavailable(err error) bool { return ClassOf(err) == ClassUnavailable }

// IsUnsupported reports whether the provider cannot answer at all.
func IsUnsupported(err error) bool { return ClassOf(err) == ClassUnsupported }

// IsPermanent reports whether retrying is pointless.
func IsPermanent(err error) bool { return ClassOf(err) == ClassPermanent }
