package defaults

import "slices"

// Scope selects which set of preferences an operation acts on.
type Scope string

const (
	// ScopeUser is the current user's preferences (the default).
	ScopeUser Scope = "user"
	// ScopeCurrentHost is the current user's ByHost preferences for this
	// machine (defaults -currentHost).
	ScopeCurrentHost Scope = "current_host"
	// ScopeSystem is machine-wide preferences in /Library/Preferences. It
	// requires running as root.
	ScopeSystem Scope = "system"
)

// Scopes lists every valid scope, in documentation order.
func Scopes() []Scope {
	return []Scope{ScopeUser, ScopeCurrentHost, ScopeSystem}
}

// RequiresRoot reports whether the scope must be used as root. Every other
// scope must be used as a non-root user, because as root it would act on
// root's own preferences.
func (s Scope) RequiresRoot() bool {
	return s == ScopeSystem
}

// Valid reports whether s is one of Scopes.
func (s Scope) Valid() bool {
	return slices.Contains(Scopes(), s)
}

// hostArgs are the arguments that come before the operation name for scopes
// that defaults selects with a flag.
func (s Scope) hostArgs() []string {
	if s == ScopeCurrentHost {
		return []string{"-currentHost"}
	}
	return nil
}
