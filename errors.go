// Copyright (c) 2020-2021 Paul Tötterma <ptman@iki.fi>. All rights reserved.

package main

import (
	"log"
	"net/http"
)

// Error is a constant error type. Nice to compare and wrap.
type Error string

// Error implements the error interface.
func (e Error) Error() string {
	return string(e)
}

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

func (e *HTTPError) polish() {
	if e.Message == "" {
		e.Message = http.StatusText(e.Code)
	}
}

// ServerHTTP implements http.Handler.
//nolint:unparam
func (e *HTTPError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.polish()

	if e.Err != nil {
		log.Printf("%+v", e.Err)
	} else {
		log.Print(e.Message)
	}

	http.Error(w, e.Message, e.Code)
}
