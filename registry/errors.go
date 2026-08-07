package registry

import (
	"context"
	"errors"
)

// Outcome identifies one terminal or operation outcome exposed by the
// Instance Registry v1 contract.
type Outcome string

const (
	OutcomeMissing          Outcome = "missing"
	OutcomeInvalid          Outcome = "invalid"
	OutcomeUnauthorized     Outcome = "unauthorized"
	OutcomeUnavailable      Outcome = "unavailable"
	OutcomeStale            Outcome = "stale"
	OutcomeWatchInterrupted Outcome = "watch_interrupted"
	OutcomeCanceled         Outcome = "canceled"
	OutcomeClosed           Outcome = "closed"
)

// OutcomeCause is an allowlisted, provider-safe classification that may
// accompany an outcome. It must never contain provider payloads, request data,
// credentials, or arbitrary transport text.
type OutcomeCause string

const (
	CauseNone                    OutcomeCause = ""
	CauseInvalidInput            OutcomeCause = "invalid_input"
	CauseUnknownOutcome          OutcomeCause = "unknown_outcome"
	CauseUnknownCause            OutcomeCause = "unknown_cause"
	CauseTerminalOutcomeRequired OutcomeCause = "terminal_outcome_required"
	CauseResourceVersionExpired  OutcomeCause = "resource_version_expired"
	CauseDeliveryOverflow        OutcomeCause = "delivery_overflow"
	CauseWatchEventTooLarge      OutcomeCause = "watch_event_too_large"
	CauseWatchEventInvalid       OutcomeCause = "watch_event_invalid"
	CauseWatchStatusError        OutcomeCause = "watch_status_error"
	CauseStreamEOF               OutcomeCause = "stream_eof"
	CauseHTTPUnauthorized        OutcomeCause = "http_unauthorized"
	CauseHTTPForbidden           OutcomeCause = "http_forbidden"
	CauseProviderUnavailable     OutcomeCause = "provider_unavailable"
	CauseRateLimited             OutcomeCause = "rate_limited"
)

// OutcomeError is an immutable, typed Instance Registry outcome error.
// Cause is limited to provider-safe classification text; it must not include
// provider payloads, credentials, or request data.
type OutcomeError struct {
	outcome Outcome
	cause   OutcomeCause
	wrapped error
}

func (e *OutcomeError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.cause == "" {
		return "instance registry: " + string(e.outcome)
	}
	return "instance registry: " + string(e.outcome) + ": " + string(e.cause)
}

// Unwrap exposes only a local cancellation/deadline cause. Providers must use
// the safe Cause text rather than exposing arbitrary transport errors.
func (e *OutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.wrapped
}

// Is makes outcome sentinels usable with errors.Is.
func (e *OutcomeError) Is(target error) bool {
	targetOutcome, ok := target.(*OutcomeError)
	return ok && e != nil && targetOutcome != nil && e.outcome == targetOutcome.outcome
}

// Outcome returns this error's v1 outcome code.
func (e *OutcomeError) Outcome() Outcome {
	if e == nil {
		return ""
	}
	return e.outcome
}

// Code is an explicit synonym for Outcome.
func (e *OutcomeError) Code() Outcome {
	return e.Outcome()
}

// Cause returns the provider-safe classification associated with this error.
func (e *OutcomeError) Cause() OutcomeCause {
	if e == nil {
		return ""
	}
	return e.cause
}

var (
	ErrMissing          = &OutcomeError{outcome: OutcomeMissing}
	ErrInvalid          = &OutcomeError{outcome: OutcomeInvalid}
	ErrUnauthorized     = &OutcomeError{outcome: OutcomeUnauthorized}
	ErrUnavailable      = &OutcomeError{outcome: OutcomeUnavailable}
	ErrStale            = &OutcomeError{outcome: OutcomeStale}
	ErrWatchInterrupted = &OutcomeError{outcome: OutcomeWatchInterrupted}
	ErrCanceled         = &OutcomeError{outcome: OutcomeCanceled}
	ErrClosed           = &OutcomeError{outcome: OutcomeClosed}
)

// NewOutcomeError creates a typed outcome error with an allowlisted safe
// cause. Unknown outcome or cause values are represented as invalid input; the
// supplied text is intentionally not retained or reflected in the result.
func NewOutcomeError(outcome Outcome, cause OutcomeCause) *OutcomeError {
	if !validOutcome(outcome) {
		return &OutcomeError{outcome: OutcomeInvalid, cause: CauseUnknownOutcome}
	}
	if !validOutcomeCause(outcome, cause) {
		return &OutcomeError{outcome: OutcomeInvalid, cause: CauseUnknownCause}
	}
	return &OutcomeError{outcome: outcome, cause: cause}
}

func newCanceledError(ctxErr error) *OutcomeError {
	if ctxErr == nil {
		ctxErr = context.Canceled
	}
	return &OutcomeError{outcome: OutcomeCanceled, wrapped: ctxErr}
}

func newInvalidError(_ string) *OutcomeError {
	return &OutcomeError{outcome: OutcomeInvalid, cause: CauseInvalidInput}
}

func validOutcome(outcome Outcome) bool {
	switch outcome {
	case OutcomeMissing, OutcomeInvalid, OutcomeUnauthorized, OutcomeUnavailable,
		OutcomeStale, OutcomeWatchInterrupted, OutcomeCanceled, OutcomeClosed:
		return true
	default:
		return false
	}
}

func validOutcomeCause(outcome Outcome, cause OutcomeCause) bool {
	switch outcome {
	case OutcomeInvalid:
		switch cause {
		case CauseNone, CauseInvalidInput, CauseUnknownOutcome, CauseUnknownCause, CauseTerminalOutcomeRequired:
			return true
		}
	case OutcomeUnauthorized:
		return cause == CauseNone || cause == CauseHTTPUnauthorized || cause == CauseHTTPForbidden
	case OutcomeUnavailable:
		return cause == CauseNone || cause == CauseProviderUnavailable || cause == CauseRateLimited
	case OutcomeStale:
		return cause == CauseNone || cause == CauseResourceVersionExpired
	case OutcomeWatchInterrupted:
		switch cause {
		case CauseNone, CauseDeliveryOverflow, CauseWatchEventTooLarge, CauseWatchEventInvalid, CauseWatchStatusError, CauseStreamEOF:
			return true
		}
	case OutcomeMissing, OutcomeCanceled, OutcomeClosed:
		return cause == CauseNone
	}
	return false
}

// OutcomeOf reports the typed v1 outcome carried by err, including through
// ordinary wrapping.
func OutcomeOf(err error) (Outcome, bool) {
	var outcomeError *OutcomeError
	if errors.As(err, &outcomeError) && outcomeError != nil {
		return outcomeError.outcome, true
	}
	return "", false
}

// IsOutcome reports whether err has the given typed v1 outcome.
func IsOutcome(err error, outcome Outcome) bool {
	if !validOutcome(outcome) {
		return false
	}
	return errors.Is(err, outcomeSentinel(outcome))
}

func IsMissing(err error) bool          { return IsOutcome(err, OutcomeMissing) }
func IsInvalid(err error) bool          { return IsOutcome(err, OutcomeInvalid) }
func IsUnauthorized(err error) bool     { return IsOutcome(err, OutcomeUnauthorized) }
func IsUnavailable(err error) bool      { return IsOutcome(err, OutcomeUnavailable) }
func IsStale(err error) bool            { return IsOutcome(err, OutcomeStale) }
func IsWatchInterrupted(err error) bool { return IsOutcome(err, OutcomeWatchInterrupted) }
func IsCanceled(err error) bool         { return IsOutcome(err, OutcomeCanceled) }
func IsClosed(err error) bool           { return IsOutcome(err, OutcomeClosed) }

func outcomeSentinel(outcome Outcome) error {
	switch outcome {
	case OutcomeMissing:
		return ErrMissing
	case OutcomeInvalid:
		return ErrInvalid
	case OutcomeUnauthorized:
		return ErrUnauthorized
	case OutcomeUnavailable:
		return ErrUnavailable
	case OutcomeStale:
		return ErrStale
	case OutcomeWatchInterrupted:
		return ErrWatchInterrupted
	case OutcomeCanceled:
		return ErrCanceled
	case OutcomeClosed:
		return ErrClosed
	default:
		return nil
	}
}

func typedTerminal(err error) error {
	if err == nil {
		return ErrClosed
	}
	var outcomeError *OutcomeError
	if errors.As(err, &outcomeError) && outcomeError != nil {
		// A source may wrap an outcome in arbitrary diagnostic text. Keep only
		// the typed code and safe cause at the public watch boundary.
		return NewOutcomeError(outcomeError.outcome, outcomeError.cause)
	}
	return NewOutcomeError(OutcomeInvalid, CauseTerminalOutcomeRequired)
}
