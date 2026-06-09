package oncall

import "fmt"

type ErrorCode string

const (
	ErrCodeInvalidInput            ErrorCode = "INVALID_INPUT"
	ErrCodeNotFound                ErrorCode = "NOT_FOUND"
	ErrCodeUnauthenticated         ErrorCode = "UNAUTHENTICATED"
	ErrCodeForbidden               ErrorCode = "FORBIDDEN"
	ErrCodeUpstreamRateLimited     ErrorCode = "UPSTREAM_RATE_LIMITED"
	ErrCodeUpstreamUnavailable     ErrorCode = "UPSTREAM_UNAVAILABLE"
	ErrCodeUpstreamTimeout         ErrorCode = "UPSTREAM_TIMEOUT"
	ErrCodeIRMPluginMissing        ErrorCode = "IRM_PLUGIN_MISSING"
	// ErrCodeOnCallPluginMissing is the dual-plugin replacement for
	// ErrCodeIRMPluginMissing. The server NEVER emits IRM_PLUGIN_MISSING;
	// that value is retained in this package as a one-minor-release
	// deprecated alias (see the JSON Schema description in
	// contracts/error_envelope.schema.json) so old clients that
	// pinned the value continue to validate.
	ErrCodeOnCallPluginMissing     ErrorCode = "ONCALL_PLUGIN_MISSING"
	ErrCodeInternal                ErrorCode = "INTERNAL"
	ErrCodeReadOnlyMode            ErrorCode = "READ_ONLY_MODE"
	ErrCodeStateTransitionRejected ErrorCode = "STATE_TRANSITION_REJECTED"
)

type Error struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Tool      string    `json:"tool"`
	Hint      *string   `json:"hint"`
	Retryable bool      `json:"retryable"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Tool, e.Message)
}

func newError(code ErrorCode, tool, message string, retryable bool) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Tool:      tool,
		Retryable: retryable,
	}
}

// NewError is the exported constructor for the Error envelope. It
// is intended for use by the oncall package's internal callers
// (the dual-plugin selector in plugin.go, future probe helpers)
// and by tests in errors_test.go; outside-package callers should
// prefer the typed NewXxxError constructors below so the code
// field stays in the documented enumerated set.
func NewError(code ErrorCode, tool, message string, retryable bool) *Error {
	return newError(code, tool, message, retryable)
}

func NewInvalidInputError(tool, message string) *Error {
	return newError(ErrCodeInvalidInput, tool, message, false)
}

func NewNotFoundError(tool, message string) *Error {
	return newError(ErrCodeNotFound, tool, message, false)
}

func NewUnauthenticatedError(tool, message string) *Error {
	return newError(ErrCodeUnauthenticated, tool, message, false)
}

func NewForbiddenError(tool, message string) *Error {
	return newError(ErrCodeForbidden, tool, message, false)
}

func NewUpstreamRateLimitedError(tool, message string) *Error {
	return newError(ErrCodeUpstreamRateLimited, tool, message, true)
}

func NewUpstreamUnavailableError(tool, message string) *Error {
	return newError(ErrCodeUpstreamUnavailable, tool, message, true)
}

func NewUpstreamTimeoutError(tool, message string) *Error {
	return newError(ErrCodeUpstreamTimeout, tool, message, true)
}

func NewInternalError(tool, message string) *Error {
	return newError(ErrCodeInternal, tool, message, false)
}

func NewReadOnlyModeError(tool, message string) *Error {
	return newError(ErrCodeReadOnlyMode, tool, message, false)
}

func NewStateTransitionRejectedError(tool, message string) *Error {
	return newError(ErrCodeStateTransitionRejected, tool, message, false)
}

// NewOnCallPluginMissingError returns an Error envelope for the
// "neither grafana-oncall-app nor grafana-irm-app is installed" case
// (FR-035). The hint names both plugin IDs so the operator can
// install at least one.
func NewOnCallPluginMissingError(tool, message string) *Error {
	return WithHint(
		newError(ErrCodeOnCallPluginMissing, tool, message, false),
		"install one of: grafana-oncall-app (legacy) or grafana-irm-app (rebranded)",
	)
}

func WithHint(err *Error, hint string) *Error {
	err.Hint = &hint
	return err
}
