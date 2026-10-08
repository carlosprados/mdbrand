// Package exit gives a failure the exit status that says who has to act: the
// author of the document, the person typing the command, or the machine.
// An agent driving mdbrand reads the status before it reads the message, and
// "fix the Markdown" and "install a font" are different next steps.
package exit

import "errors"

// The exit statuses, part of mdbrand's stability contract.
const (
	OK = 0
	// Defect: the document or its bundle would make a defective PDF, and the
	// message names the fix. Also anything not classified otherwise.
	Defect = 1
	// Usage: the command line is wrong — a flag, an argument, a file that
	// is not there.
	Usage = 2
	// Environment: something the document asks for is not on this machine —
	// a tool, a font, a bundle. `mdbrand doctor` says how to get it.
	Environment = 3
)

type classified struct {
	code int
	err  error
}

func (c *classified) Error() string { return c.err.Error() }
func (c *classified) Unwrap() error { return c.err }

// AsUsage marks err as a mistake on the command line. nil stays nil.
func AsUsage(err error) error { return mark(Usage, err) }

// AsEnvironment marks err as something missing from the machine. nil stays nil.
func AsEnvironment(err error) error { return mark(Environment, err) }

func mark(code int, err error) error {
	if err == nil {
		return nil
	}
	return &classified{code: code, err: err}
}

// Code is the exit status for err: OK for nil, the innermost mark when there
// is one, and Defect otherwise. The innermost wins because it was set where
// the cause was known; a wrapper further out knows less.
func Code(err error) int {
	if err == nil {
		return OK
	}
	code := Defect
	for e := err; e != nil; e = errors.Unwrap(e) {
		if c, ok := e.(*classified); ok {
			code = c.code
		}
	}
	return code
}
