package defaults

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// GlobalDomain is the domain every application falls back to. defaults
// domains doesn't list it, so SupportedDomains always adds it.
const GlobalDomain = "NSGlobalDomain"

// maxDomainLen bounds domain names. Real ones are far shorter; this only stops
// absurd input.
const maxDomainLen = 255

// Domains implements Runner.
//
// For the user scopes it parses `defaults [-currentHost] domains`, which
// prints one line of names separated by ", ". That is the one place the
// provider parses defaults' text output. A name that itself contains ", "
// can't be told apart and comes out split; SupportedDomains drops the pieces
// only if they aren't valid names on their own.
//
// For ScopeSystem, defaults has no listing, so it reads the file names (never
// the contents) of the regular *.plist files in SystemPreferencesDir.
func (r *execRunner) Domains(ctx context.Context, scope Scope) ([]string, error) {
	if scope == ScopeSystem {
		return r.systemDomains()
	}
	if !scope.Valid() {
		return nil, fmt.Errorf("unknown scope %q", scope)
	}

	out, err := r.run(ctx, "domains", "", "", append(scope.hostArgs(), "domains")...)
	if err != nil {
		return nil, err
	}
	return parseDomainList(string(out)), nil
}

func (r *execRunner) systemDomains() ([]string, error) {
	entries, err := fs.ReadDir(r.systemFS, ".")
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", SystemPreferencesDir, err)
	}
	var names []string

	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if name, ok := strings.CutSuffix(e.Name(), ".plist"); ok {
			names = append(names, name)
		}
	}
	return names, nil
}

func parseDomainList(out string) []string {
	var names []string
	for name := range strings.SplitSeq(out, ", ") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// SupportedDomains turns a raw listing from Runner.Domains into what the
// provider exposes: only names IsSupportedDomain accepts, plus GlobalDomain,
// de-duplicated and sorted.
func SupportedDomains(raw []string) []string {
	out := []string{GlobalDomain}
	for _, name := range raw {
		if IsSupportedDomain(name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// IsSupportedDomain reports whether name is a domain the provider will read
// or write.
//
// Supported names start with a letter or digit, end with a letter, digit, _
// or -, and otherwise contain only letters, digits, '.', '_', '-' and spaces,
// with no "..". That covers bundle identifiers (com.apple.dock),
// NSGlobalDomain, and the undotted names some system agents use
// (loginwindow). It rules out everything defaults would treat as something
// other than a domain name: paths (anything with '/'), flags (a leading '-'),
// and hidden files (a leading '.').
//
// The hidden-name rule deliberately excludes .GlobalPreferences (an alias of
// NSGlobalDomain) and .GlobalPreferences_m (a private, signed copy of the
// language and region keys that macOS maintains). See DEVELOPMENT.md before
// adding any exception.
func IsSupportedDomain(name string) bool {
	if name == "" || len(name) > maxDomainLen || strings.Contains(name, "..") {
		return false
	}
	for i, c := range name {
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		underDash := c == '_' || c == '-'
		switch {
		case i == 0 && !alnum:
			return false
		case i == len(name)-1 && !alnum && !underDash:
			return false
		case !alnum && c != '.' && !underDash && c != ' ':
			return false
		}
	}
	return true
}
