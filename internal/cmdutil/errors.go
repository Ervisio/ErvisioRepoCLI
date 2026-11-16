package cmdutil

import (
	"errors"
	"fmt"
)

// FlagError is a usage error: main prints it followed by the usage.
type FlagError struct {
	Err error
	Cmd *Command
}

func (e *FlagError) Error() string { return e.Err.Error() }
func (e *FlagError) Unwrap() error { return e.Err }

// FlagErrorf builds a FlagError.
func FlagErrorf(format string, a ...any) error {
	return &FlagError{Err: fmt.Errorf(format, a...)}
}

// ErrSilent exits 1 without printing anything more.
var ErrSilent = errors.New("SilentError")

// ErrCancel is a prompt the user aborted (exit 2).
var ErrCancel = errors.New("CancelError")

// AuthError means no credentials are available (exit 4).
type AuthError struct{ Host string }

func (e *AuthError) Error() string {
	if e.Host != "" {
		return fmt.Sprintf("You are not logged in to %s. To log in, run:  eg auth login --hostname %s\n"+
			"Alternatively, populate the EG_TOKEN environment variable with an ErvisioRepo personal access token.", e.Host, e.Host)
	}
	return "To get started with Ervisio CLI, please run:  eg auth login\n" +
		"Alternatively, populate the EG_HOST and EG_TOKEN environment variables with your ErvisioRepo server and a personal access token."
}

// UnsupportedError is a feature the ErvisioRepo server does not offer yet.
type UnsupportedError struct {
	What     string
	Endpoint string
}

func (e *UnsupportedError) Error() string {
	msg := e.What + " is not supported by this ErvisioRepo server yet"
	if e.Endpoint != "" {
		msg += " (missing " + e.Endpoint + ")"
	}
	return msg
}

// Unsupported builds an UnsupportedError.
func Unsupported(what, endpoint string) error {
	return &UnsupportedError{What: what, Endpoint: endpoint}
}

// ErrPending exits 8 without printing anything more (checks pending).
var ErrPending = errors.New("PendingError")
