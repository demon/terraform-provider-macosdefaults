package defaults

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var (
	// ErrNotFound means the domain or key doesn't exist.
	ErrNotFound = errors.New("not found")
	// ErrPermission means macOS refused access, usually because the process
	// running Terraform lacks Full Disk Access for a protected domain, or
	// isn't root for a system domain.
	ErrPermission = errors.New("permission denied")
)

// Error is a failed defaults operation.
type Error struct {
	// Op is the defaults operation, e.g. "write".
	Op string
	// Domain and Key identify what the operation targeted, when it targeted
	// anything.
	Domain, Key string
	// ExitCode is the process exit status, or -1 if it never ran or was killed.
	ExitCode int
	// Stderr is the trimmed standard error output.
	Stderr string

	kind error // ErrNotFound, ErrPermission, or nil
	err  error
}

func newError(op, domain, key string, err error, stderr []byte) *Error {
	e := &Error{
		Op:       op,
		Domain:   domain,
		Key:      key,
		ExitCode: -1,
		Stderr:   strings.TrimSpace(string(stderr)),
		err:      err,
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		e.ExitCode = exitErr.ExitCode()
	}
	e.kind = classify(e.Stderr)
	return e
}

// classify maps defaults' stderr onto an error class. The not-found messages
// were observed on macOS 27:
//
//	Error: Could not find key 'k' in domain 'd'.
//	Error: Domain 'd' not found.
//
// The permission message is what macOS prints for EPERM; it hasn't been
// observed from defaults itself yet.
func classify(stderr string) error {
	switch {
	case strings.Contains(stderr, "Could not find key"),
		strings.Contains(stderr, "not found"):
		return ErrNotFound
	case strings.Contains(stderr, "Operation not permitted"),
		strings.Contains(stderr, "Permission denied"):
		return ErrPermission
	}
	return nil
}

func (e *Error) Error() string {
	msg := "defaults " + e.Op
	if e.Domain != "" {
		msg += " " + e.Domain
	}
	if e.Key != "" {
		msg += " " + e.Key
	}
	msg += " failed"
	if e.ExitCode >= 0 {
		msg += fmt.Sprintf(" with exit status %d", e.ExitCode)
	}
	if e.Stderr != "" {
		return msg + ": " + e.Stderr
	}
	return msg + ": " + e.err.Error()
}

// Unwrap exposes both the error class (for errors.Is with ErrNotFound or
// ErrPermission) and the underlying process error.
func (e *Error) Unwrap() []error {
	if e.kind == nil {
		return []error{e.err}
	}
	return []error{e.kind, e.err}
}
