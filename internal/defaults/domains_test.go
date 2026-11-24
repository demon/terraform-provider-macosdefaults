package defaults

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

type call struct {
	name string
	args []string
}

// stubExec records each call and replies with the given output.
func stubExec(calls *[]call, stdout, stderr string, err error) execFunc {
	return func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		*calls = append(*calls, call{name: name, args: args})
		return []byte(stdout), []byte(stderr), err
	}
}

func TestDomainsArgs(t *testing.T) {
	tests := []struct {
		scope    Scope
		wantArgs []string
	}{
		{ScopeUser, []string{"domains"}},
		{ScopeCurrentHost, []string{"-currentHost", "domains"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.scope), func(t *testing.T) {
			var calls []call
			r := &execRunner{exec: stubExec(&calls, "com.apple.dock, loginwindow\n", "", nil)}

			got, err := r.Domains(context.Background(), tt.scope)
			if err != nil {
				t.Fatalf("Domains: %v", err)
			}
			if len(calls) != 1 {
				t.Fatalf("got %d exec calls, want 1", len(calls))
			}
			if calls[0].name != Binary {
				t.Errorf("ran %q, want %q", calls[0].name, Binary)
			}
			if !slices.Equal(calls[0].args, tt.wantArgs) {
				t.Errorf("args = %q, want %q", calls[0].args, tt.wantArgs)
			}
			if want := []string{"com.apple.dock", "loginwindow"}; !slices.Equal(got, want) {
				t.Errorf("Domains = %q, want %q", got, want)
			}
		})
	}
}

func TestDomainsUnknownScope(t *testing.T) {
	var calls []call
	r := &execRunner{exec: stubExec(&calls, "", "", nil)}

	if _, err := r.Domains(context.Background(), Scope("bogus")); err == nil {
		t.Fatal("Domains with unknown scope: want error, got nil")
	}
	if len(calls) != 0 {
		t.Errorf("ran defaults %d times for an unknown scope, want 0", len(calls))
	}
}

func TestDomainsCommandFailure(t *testing.T) {
	var calls []call
	exitErr := &exec.ExitError{}
	r := &execRunner{exec: stubExec(&calls, "", "  something broke\n", exitErr)}

	_, err := r.Domains(context.Background(), ScopeUser)

	var dErr *Error
	if !errors.As(err, &dErr) {
		t.Fatalf("error is %T (%v), want *defaults.Error", err, err)
	}
	if dErr.Op != "domains" {
		t.Errorf("Op = %q, want %q", dErr.Op, "domains")
	}
	if dErr.Stderr != "something broke" {
		t.Errorf("Stderr = %q, want trimmed %q", dErr.Stderr, "something broke")
	}
	if !errors.Is(err, exitErr) {
		t.Error("error does not unwrap to the underlying exec error")
	}
	if !strings.Contains(err.Error(), "something broke") {
		t.Errorf("Error() = %q, want it to include stderr", err.Error())
	}
}

func TestDomainsSystem(t *testing.T) {
	var calls []call
	r := &execRunner{
		exec: stubExec(&calls, "", "", nil),
		systemFS: fstest.MapFS{
			"com.apple.loginwindow.plist":        {},
			"com.apple.dock.plist":               {},
			".GlobalPreferences.plist":           {},
			"notes.txt":                          {},
			"Audio":                              {Mode: 0o755 | fs.ModeDir},
			"com.apple.apsd.launchd":             {Mode: 0o755 | fs.ModeDir},
			"com.example.linked.plist":           {Mode: fs.ModeSymlink},
			"Audio/com.apple.nested.plist":       {},
			"com.example.name with spaces.plist": {},
		},
	}

	got, err := r.Domains(context.Background(), ScopeSystem)
	if err != nil {
		t.Fatalf("Domains: %v", err)
	}
	if len(calls) != 0 {
		t.Errorf("system listing ran defaults %d times, want 0", len(calls))
	}
	slices.Sort(got)
	want := []string{".GlobalPreferences", "com.apple.dock", "com.apple.loginwindow", "com.example.name with spaces"}
	if !slices.Equal(got, want) {
		t.Errorf("Domains = %q, want %q", got, want)
	}
}

func TestDomainsSystemReadError(t *testing.T) {
	r := &execRunner{systemFS: unreadableFS{}}

	_, err := r.Domains(context.Background(), ScopeSystem)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("error = %v, want it to wrap fs.ErrPermission", err)
	}
	if !strings.Contains(err.Error(), SystemPreferencesDir) {
		t.Errorf("Error() = %q, want it to name %s", err.Error(), SystemPreferencesDir)
	}
}

// unreadableFS fails every Open with a permission error.
type unreadableFS struct{}

func (unreadableFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

func TestParseDomainList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"\n", nil},
		{"com.apple.dock\n", []string{"com.apple.dock"}},
		{"a.b, c.d, e.f\n", []string{"a.b", "c.d", "e.f"}},
		{"com.topazlabs.Topaz Video, pbs\n", []string{"com.topazlabs.Topaz Video", "pbs"}},
	}
	for _, tt := range tests {
		if got := parseDomainList(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("parseDomainList(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsSupportedDomain(t *testing.T) {
	supported := []string{
		"com.apple.dock",
		"NSGlobalDomain",
		"loginwindow",
		"pbs",
		"com.topazlabs.Topaz Video",
		"V380-Ori.Astris",
		"diagnostics_agent",
		"com.example.trailing-",
		"com.example.trailing_",
		"3rd.party",
		strings.Repeat("a", maxDomainLen),
	}
	unsupported := []string{
		"",
		"-g",
		"-globalDomain",
		".GlobalPreferences",
		".GlobalPreferences_m",
		"/Library/Preferences/com.apple.dock",
		"../etc/passwd",
		"com..apple",
		"com.apple.dock.",
		"com.apple.dock ",
		" com.apple.dock",
		"com.apple/dock",
		"com.apple.dock\n",
		"com.apple:dock",
		"com.ünicode",
		strings.Repeat("a", maxDomainLen+1),
	}
	for _, name := range supported {
		if !IsSupportedDomain(name) {
			t.Errorf("IsSupportedDomain(%q) = false, want true", name)
		}
	}
	for _, name := range unsupported {
		if IsSupportedDomain(name) {
			t.Errorf("IsSupportedDomain(%q) = true, want false", name)
		}
	}
}

func TestSupportedDomains(t *testing.T) {
	got := SupportedDomains([]string{
		"com.apple.dock",
		".GlobalPreferences_m",
		"loginwindow",
		"com.apple.dock",
		"/etc/passwd",
		"NSGlobalDomain",
		"ai.example",
	})
	want := []string{"NSGlobalDomain", "ai.example", "com.apple.dock", "loginwindow"}
	if !slices.Equal(got, want) {
		t.Errorf("SupportedDomains = %q, want %q", got, want)
	}

	if got := SupportedDomains(nil); !slices.Equal(got, []string{GlobalDomain}) {
		t.Errorf("SupportedDomains(nil) = %q, want only %q", got, GlobalDomain)
	}
}
