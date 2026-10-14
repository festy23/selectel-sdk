package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type ErrorClass string

const (
	ErrorClassBadRequest         ErrorClass = "bad_request"
	ErrorClassNotAuthenticated   ErrorClass = "not_authenticated"
	ErrorClassForbidden          ErrorClass = "forbidden"
	ErrorClassNotFound           ErrorClass = "not_found"
	ErrorClassConflict           ErrorClass = "conflict"
	ErrorClassServer             ErrorClass = "server"
	ErrorClassUnexpectedResponse ErrorClass = "unexpected_response"
	ErrorClassUnclassified       ErrorClass = "unclassified"
)

type ClientError struct{ Err error }

func (err *ClientError) Error() string { return "vpc client error: " + err.Err.Error() }
func (err *ClientError) Unwrap() error { return err.Err }

type TransportError struct{ Err error }

func (err *TransportError) Error() string { return "vpc transport error: " + err.Err.Error() }
func (err *TransportError) Unwrap() error { return err.Err }

type Error struct {
	StatusCode int
	Type       string
	Message    string
	Detail     string
	Class      ErrorClass
	RawBody    []byte
}

func (err *Error) Error() string {
	if err.Message != "" {
		return fmt.Sprintf("vpc API error (%d, %s): %s", err.StatusCode, err.Type, err.Message)
	}

	return fmt.Sprintf("vpc API error (%d, %s)", err.StatusCode, err.Type)
}

type UnexpectedResponseError struct {
	StatusCode int
	RawBody    []byte
	Err        error
}

func (err *UnexpectedResponseError) Error() string {
	return fmt.Sprintf("unexpected vpc API response (%d): %v", err.StatusCode, err.Err)
}
func (err *UnexpectedResponseError) Unwrap() error { return err.Err }

func IsErrorClass(err error, class ErrorClass) bool {
	if apiErr, ok := errors.AsType[*Error](err); ok {
		return apiErr.Class == class
	}
	var unexpectedErr *UnexpectedResponseError

	return class == ErrorClassUnexpectedResponse && errors.As(err, &unexpectedErr)
}

func NewAPIError(statusCode int, body []byte) *Error {
	var envelope struct {
		NeutronError struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Detail  string `json:"detail"`
		} `json:"NeutronError"`
	}
	_ = json.Unmarshal(body, &envelope)

	apiErr := &Error{
		StatusCode: statusCode,
		Type:       envelope.NeutronError.Type,
		Message:    envelope.NeutronError.Message,
		Detail:     envelope.NeutronError.Detail,
		RawBody:    append([]byte(nil), body...),
	}
	apiErr.Class = classifyAPIError(apiErr)

	return apiErr
}

func classifyAPIError(apiErr *Error) ErrorClass {
	switch apiErr.StatusCode {
	case http.StatusBadRequest:
		return ErrorClassBadRequest
	case http.StatusUnauthorized:
		return ErrorClassNotAuthenticated
	case http.StatusForbidden:
		return ErrorClassForbidden
	case http.StatusNotFound:
		return ErrorClassNotFound
	case http.StatusConflict:
		return ErrorClassConflict
	default:
		if apiErr.StatusCode >= http.StatusInternalServerError {
			return ErrorClassServer
		}

		return ErrorClassUnclassified
	}
}
