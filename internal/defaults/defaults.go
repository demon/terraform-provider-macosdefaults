// Package defaults wraps macOS's /usr/bin/defaults. It is the only package in
// the provider that executes processes.
//
// Commands are always run by absolute path with exec.CommandContext and an
// argument slice: never through a shell and never resolved through $PATH. See
// DEVELOPMENT.md for the full set of conventions.
package defaults

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"sync"

	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
)

// Binary is the absolute path of the defaults tool. It is never looked up
// through $PATH.
const Binary = "/usr/bin/defaults"

// SystemPreferencesDir holds machine-wide preference domains.
const SystemPreferencesDir = "/Library/Preferences"

// Runner executes defaults(1) operations against the preferences system.
//
// Operations are added here as resources and data sources need them, each
// with a matching implementation in the defaultstest fake used by unit tests.
type Runner interface {
	// Domains lists the domain names present in scope, as the system reports
	// them: unfiltered and unsorted. Use SupportedDomains to turn the result
	// into what the provider exposes.
	Domains(ctx context.Context, scope Scope) ([]string, error)

	// Export returns the domain as the XML property list `defaults export`
	// prints, unparsed. A domain that doesn't exist exports as an empty dict,
	// not an error. Decode it with plistvalue.DecodeDomain.
	Export(ctx context.Context, scope Scope, domain string) ([]byte, error)

	// Write sets one key, replacing any existing value whatever its type.
	Write(ctx context.Context, scope Scope, domain, key string, v plistvalue.Value) error

	// Delete removes one key. A missing key or domain is an error wrapping
	// ErrNotFound. It never removes a whole domain.
	Delete(ctx context.Context, scope Scope, domain, key string) error
}

// New returns the Runner backed by the real defaults binary.
func New() Runner {
	return &execRunner{
		exec:     runCommand,
		systemFS: os.DirFS(SystemPreferencesDir),
	}
}

// execFunc runs a command and returns its stdout and stderr. A non-zero exit
// is reported as an *exec.ExitError.
type execFunc func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

// execRunner runs Binary as the current user. Its dependencies are fields so
// unit tests can check the exact arguments without running a process.
type execRunner struct {
	exec     execFunc
	systemFS fs.FS

	// locks serialises writes and deletes per (scope, domain), so errors
	// stay attributable under Terraform's parallelism. cfprefsd already
	// handles the concurrency itself.
	locks sync.Map // string -> *sync.Mutex
}

func (r *execRunner) lock(scope Scope, domain string) (unlock func()) {
	m, _ := r.locks.LoadOrStore(string(scope)+"\x00"+domain, &sync.Mutex{})
	mu := m.(*sync.Mutex) //nolint:forcetypeassert // only *sync.Mutex is stored
	mu.Lock()
	return mu.Unlock
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// run executes Binary with args and wraps any failure in an *Error that
// names op, domain and key.
func (r *execRunner) run(ctx context.Context, op, domain, key string, args ...string) ([]byte, error) {
	stdout, stderr, err := r.exec(ctx, Binary, args...)
	if err != nil {
		return nil, newError(op, domain, key, err, stderr)
	}
	return stdout, nil
}
