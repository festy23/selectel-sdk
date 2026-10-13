package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Code string

const (
	InvalidArgument        Code = "invalid_argument"
	ScopeMismatch          Code = "scope_mismatch"
	CredentialsUnavailable Code = "credentials_unavailable"
	Unauthenticated        Code = "unauthenticated"
	PermissionDenied       Code = "permission_denied"
	NotFound               Code = "not_found"
	Conflict               Code = "conflict"
	RateLimited            Code = "rate_limited"
	CapabilityUnavailable  Code = "capability_unavailable"
	EndpointUnavailable    Code = "endpoint_unavailable"
	ProviderError          Code = "provider_error"
	TransportError         Code = "transport_error"
	ProtocolError          Code = "protocol_error"
	ResponseLimit          Code = "response_limit"
	Incomplete             Code = "incomplete"
	DeadlineExceeded       Code = "deadline_exceeded"
	Canceled               Code = "canceled"
	UnknownOutcome         Code = "unknown_outcome"
	RecoveryUnavailable    Code = "recovery_unavailable"
	OperationFailed        Code = "operation_failed"
	StateUnknown           Code = "state_unknown"
)

var messages = map[Code]string{
	InvalidArgument: "Invalid argument", ScopeMismatch: "Scope mismatch",
	CredentialsUnavailable: "Credentials unavailable", Unauthenticated: "Authentication failed",
	PermissionDenied: "Permission denied", NotFound: "Resource not found",
	Conflict: "Resource conflict", RateLimited: "Rate limited",
	CapabilityUnavailable: "Capability unavailable", EndpointUnavailable: "Endpoint unavailable",
	ProviderError: "Provider request failed", TransportError: "Transport failed",
	ProtocolError: "Invalid provider response", ResponseLimit: "Response limit exceeded",
	Incomplete: "Result incomplete", DeadlineExceeded: "Deadline exceeded", Canceled: "Call canceled",
	UnknownOutcome: "Mutation outcome unknown", RecoveryUnavailable: "Recovery unavailable",
	OperationFailed: "Operation failed", StateUnknown: "Provider state unknown",
}

// SDKError deliberately excludes provider messages, URLs, bodies and raw causes.
// Its cause, when present, is another sanitized SDK error or a context sentinel.
type SDKError struct {
	Code          Code     `json:"code"`
	CauseCode     Code     `json:"cause_code,omitempty"`
	OperationID   string   `json:"operation_id,omitempty"`
	Scope         Scope    `json:"scope"`
	EndpointID    string   `json:"endpoint_id,omitempty"`
	HTTPStatus    int      `json:"http_status,omitempty"`
	ProviderCode  string   `json:"provider_code,omitempty"`
	RequestIDs    []string `json:"request_ids,omitempty"`
	Receipt       *Receipt `json:"receipt,omitempty"`
	RetryableRead bool     `json:"retryable_read"`
	Reason        string   `json:"reason,omitempty"`
	cause         error
}

func NewError(code Code) *SDKError { return &SDKError{Code: code} }
func (e *SDKError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if m, ok := messages[e.Code]; ok {
		return m
	}
	return "SDK request failed"
}
func (e *SDKError) String() string             { return e.Error() }
func (e *SDKError) GoString() string           { return e.Error() }
func (e *SDKError) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, e.Error()) }
func (e *SDKError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
func (e *SDKError) Is(target error) bool {
	t, ok := target.(*SDKError)
	return ok && e != nil && t != nil && e.Code == t.Code
}

func (e *SDKError) MarshalJSON() ([]byte, error) {
	type public SDKError
	return json.Marshal(struct {
		*public
		Message string `json:"message"`
	}{(*public)(e), e.Error()})
}

// ContextError preserves errors.Is while never wrapping a raw URL/provider error.
func ContextError(err error) *SDKError {
	if errors.Is(err, context.Canceled) {
		return &SDKError{Code: Canceled, cause: context.Canceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &SDKError{Code: DeadlineExceeded, cause: context.DeadlineExceeded}
	}
	return NewError(TransportError)
}

func ErrorCode(err error) Code {
	var sdk *SDKError
	if errors.As(err, &sdk) {
		return sdk.Code
	}
	if errors.Is(err, context.Canceled) {
		return Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return DeadlineExceeded
	}
	if err != nil {
		return TransportError
	}
	return ""
}
