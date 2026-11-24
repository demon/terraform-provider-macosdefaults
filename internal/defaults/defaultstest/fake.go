// Package defaultstest provides an in-memory defaults.Runner for unit tests.
package defaultstest

import (
	"context"
	"fmt"
	"sync"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
)

var _ defaults.Runner = (*Fake)(nil)

// Fake is an in-memory defaults.Runner. The zero value has no domains in any
// scope. It is safe for concurrent use. Set the exported fields before use;
// use the methods afterwards.
type Fake struct {
	// DomainsByScope is what Domains returns for each scope.
	DomainsByScope map[defaults.Scope][]string
	// DomainsErr, if set, is returned by every Domains call.
	DomainsErr error
	// ExportErr, WriteErr and DeleteErr, if set, are returned by every call to
	// that operation.
	ExportErr, WriteErr, DeleteErr error

	// DomainsCalls records the scope of each Domains call.
	DomainsCalls []defaults.Scope

	mu     sync.Mutex
	prefs  map[defaults.Scope]map[string]map[string]plistvalue.Value
	writes int
}

// Domains implements defaults.Runner.
func (f *Fake) Domains(_ context.Context, scope defaults.Scope) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DomainsCalls = append(f.DomainsCalls, scope)
	if f.DomainsErr != nil {
		return nil, f.DomainsErr
	}
	return append([]string(nil), f.DomainsByScope[scope]...), nil
}

// Export implements defaults.Runner. Like the real thing, a missing domain
// exports as an empty dict.
func (f *Fake) Export(_ context.Context, scope defaults.Scope, domain string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ExportErr != nil {
		return nil, f.ExportErr
	}
	return plistvalue.EncodeDocument(plistvalue.Dict(f.prefs[scope][domain]))
}

// Write implements defaults.Runner.
func (f *Fake) Write(_ context.Context, scope defaults.Scope, domain, key string, v plistvalue.Value) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.WriteErr != nil {
		return f.WriteErr
	}
	f.writes++
	f.set(scope, domain, key, v)
	return nil
}

// Delete implements defaults.Runner. A missing key wraps defaults.ErrNotFound.
func (f *Fake) Delete(_ context.Context, scope defaults.Scope, domain, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DeleteErr != nil {
		return f.DeleteErr
	}
	if _, ok := f.prefs[scope][domain][key]; !ok {
		return fmt.Errorf("fake: key %q in domain %q: %w", key, domain, defaults.ErrNotFound)
	}
	delete(f.prefs[scope][domain], key)
	return nil
}

// Set stores a value directly, as if another program had written it. It
// doesn't count as a write.
func (f *Fake) Set(scope defaults.Scope, domain, key string, v plistvalue.Value) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set(scope, domain, key, v)
}

func (f *Fake) set(scope defaults.Scope, domain, key string, v plistvalue.Value) {
	if f.prefs == nil {
		f.prefs = map[defaults.Scope]map[string]map[string]plistvalue.Value{}
	}
	if f.prefs[scope] == nil {
		f.prefs[scope] = map[string]map[string]plistvalue.Value{}
	}
	if f.prefs[scope][domain] == nil {
		f.prefs[scope][domain] = map[string]plistvalue.Value{}
	}
	f.prefs[scope][domain][key] = v
}

// Get returns a stored value.
func (f *Fake) Get(scope defaults.Scope, domain, key string) (plistvalue.Value, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.prefs[scope][domain][key]
	return v, ok
}

// Remove deletes a stored value directly, as if another program had. It is a
// no-op if the key is missing.
func (f *Fake) Remove(scope defaults.Scope, domain, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.prefs[scope][domain], key)
}

// Writes reports how many Write calls have succeeded.
func (f *Fake) Writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}
