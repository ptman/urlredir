// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"log/slog"
	"net/http"
)

// Error is a constant error type. Nice to compare and wrap.
type Error string

// Error implements the error interface.
func (e Error) Error() string {
	return string(e)
}

// Sentinel errors.
const (
	ErrFailedRollback Error = "failed rollback"
	ErrInvalidIP      Error = "invalid IP"
	ErrInvalidURL     Error = "invalid URL"
	ErrMissingName    Error = "missing name"
	ErrMissingURL     Error = "missing URL"
	ErrMissingUser    Error = "missing user"
	ErrNoTx           Error = "no tx"
	ErrUnknown        Error = "unknown error"
)

// HTTPError is an error returned over the network.
type HTTPError struct {
	Code    int
	Err     error
	Message string
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	return e.Message
}

// ServerHTTP implements http.Handler.
func (e *HTTPError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.polish()

	slog.Error("error",
		slog.String("method", r.Method),
		slog.String("url", r.RequestURI),
		slog.Int("status", e.Code),
		slog.String("remote", r.RemoteAddr),
		slog.String("message", e.Message),
		slog.Any("err", e.Err),
	)

	http.Error(w, e.Message, e.Code)
}

func (e *HTTPError) polish() {
	if e.Message == "" {
		e.Message = http.StatusText(e.Code)
	}
}
