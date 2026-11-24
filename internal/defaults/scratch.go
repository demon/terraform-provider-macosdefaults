package defaults

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScratchDomainPrefix starts every domain an acceptance test may write to.
// RemoveScratchDomain refuses anything else.
const ScratchDomainPrefix = "tf-acc."

// RemoveScratchDomain deletes an acceptance-test scratch domain entirely.
//
// It is the only code in the provider that removes a whole domain, and it
// exists only for test cleanup: it refuses any domain that doesn't start with
// ScratchDomainPrefix. `defaults delete <domain>` empties the domain but
// leaves an empty plist file behind (so the domain keeps showing up in
// `defaults domains`), so once the domain exports as empty this also removes
// those leftover files.
func RemoveScratchDomain(ctx context.Context, scope Scope, domain string) error {
	if !strings.HasPrefix(domain, ScratchDomainPrefix) || !IsSupportedDomain(domain) {
		return fmt.Errorf("refusing to remove %q: not a %s scratch domain", domain, ScratchDomainPrefix)
	}
	host, name, err := target(scope, domain)
	if err != nil {
		return err
	}

	r := &execRunner{exec: runCommand}
	if _, err := r.run(ctx, "delete", domain, "", append(host, "delete", name)...); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}

	var pattern string
	switch scope {
	case ScopeUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("finding home directory: %w", err)
		}
		pattern = filepath.Join(home, "Library", "Preferences", domain+".plist")
	case ScopeCurrentHost:
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("finding home directory: %w", err)
		}
		pattern = filepath.Join(home, "Library", "Preferences", "ByHost", domain+".*.plist")
	case ScopeSystem:
		pattern = filepath.Join(SystemPreferencesDir, domain+".plist")
	}
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("finding leftover files for %s: %w", domain, err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing leftover %s: %w", f, err)
		}
	}
	return nil
}
