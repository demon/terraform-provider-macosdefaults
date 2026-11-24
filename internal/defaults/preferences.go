package defaults

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
)

// systemGlobalFile is the system-scope file behind NSGlobalDomain. It isn't
// named after the domain, so target special-cases it.
const systemGlobalFile = ".GlobalPreferences"

// IsSupportedKey reports whether key can be read or written. Keys are
// otherwise free-form (spaces, dots, colons and slashes all occur in real
// ones), but they can't be empty, can't start with '-' (defaults would read
// them as a flag) and can't contain control characters.
func IsSupportedKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "-") {
		return false
	}
	if plistvalue.CheckXMLText(key) != nil {
		return false
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// target turns (scope, domain) into the defaults arguments that select it:
// optional host flags, then the domain name or, for ScopeSystem, the path
// under SystemPreferencesDir. It is the only place that path is built.
func target(scope Scope, domain string) (hostArgs []string, name string, err error) {
	if !scope.Valid() {
		return nil, "", fmt.Errorf("unknown scope %q", scope)
	}
	if !IsSupportedDomain(domain) {
		return nil, "", fmt.Errorf("unsupported domain name %q", domain)
	}
	if scope != ScopeSystem {
		return scope.hostArgs(), domain, nil
	}
	file := domain
	if domain == GlobalDomain {
		file = systemGlobalFile
	}
	return nil, SystemPreferencesDir + "/" + file, nil
}

// Export implements Runner.
func (r *execRunner) Export(ctx context.Context, scope Scope, domain string) ([]byte, error) {
	host, name, err := target(scope, domain)
	if err != nil {
		return nil, err
	}
	return r.run(ctx, "export", domain, "", append(host, "export", name, "-")...)
}

// Write implements Runner.
func (r *execRunner) Write(ctx context.Context, scope Scope, domain, key string, v plistvalue.Value) error {
	host, name, err := target(scope, domain)
	if err != nil {
		return err
	}
	if !IsSupportedKey(key) {
		return fmt.Errorf("unsupported key %q", key)
	}
	xml, err := plistvalue.EncodeXML(v)
	if err != nil {
		return fmt.Errorf("encoding value for %s %s: %w", domain, key, err)
	}

	unlock := r.lock(scope, domain)
	defer unlock()
	_, err = r.run(ctx, "write", domain, key, append(host, "write", name, key, xml)...)
	return err
}

// Delete implements Runner.
func (r *execRunner) Delete(ctx context.Context, scope Scope, domain, key string) error {
	host, name, err := target(scope, domain)
	if err != nil {
		return err
	}
	if !IsSupportedKey(key) {
		return fmt.Errorf("unsupported key %q", key)
	}

	unlock := r.lock(scope, domain)
	defer unlock()
	_, err = r.run(ctx, "delete", domain, key, append(host, "delete", name, key)...)
	return err
}
