// Copyright (c) 2020 Paul Tötterma <ptman@iki.fi>. All rights reserved.

package main

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
