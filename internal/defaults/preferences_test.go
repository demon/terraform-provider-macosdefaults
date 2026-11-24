package defaults

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
)

func TestTarget(t *testing.T) {
	tests := []struct {
		scope    Scope
		domain   string
		wantHost []string
		wantName string
	}{
		{ScopeUser, "com.apple.dock", nil, "com.apple.dock"},
		{ScopeUser, "NSGlobalDomain", nil, "NSGlobalDomain"},
		{ScopeCurrentHost, "com.apple.controlcenter", []string{"-currentHost"}, "com.apple.controlcenter"},
		{ScopeSystem, "com.apple.loginwindow", nil, "/Library/Preferences/com.apple.loginwindow"},
		{ScopeSystem, "NSGlobalDomain", nil, "/Library/Preferences/.GlobalPreferences"},
	}
	for _, tt := range tests {
		host, name, err := target(tt.scope, tt.domain)
		if err != nil {
			t.Errorf("target(%s, %s): %v", tt.scope, tt.domain, err)
			continue
		}
		if !slices.Equal(host, tt.wantHost) || name != tt.wantName {
			t.Errorf("target(%s, %s) = %q, %q; want %q, %q", tt.scope, tt.domain, host, name, tt.wantHost, tt.wantName)
		}
	}

	for _, bad := range []struct {
		scope  Scope
		domain string
	}{
		{ScopeSystem, "../../etc/passwd"},
		{ScopeSystem, "com.apple/../../x"},
		{ScopeUser, "-g"},
		{ScopeUser, "/tmp/x"},
		{Scope("nope"), "com.apple.dock"},
	} {
		if _, name, err := target(bad.scope, bad.domain); err == nil {
			t.Errorf("target(%s, %q) = %q, want error", bad.scope, bad.domain, name)
		}
	}
}

func TestIsSupportedKey(t *testing.T) {
	for _, k := range []string{"autohide", "NSWindow Frame Main", "com.apple.keyboard.fnState", "a:b/c", "ünïcode", "-"[1:] + "x-y"} {
		if !IsSupportedKey(k) {
			t.Errorf("IsSupportedKey(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"", "-string", "-", "tab\there", "new\nline", "nul\x00", "bad\xff"} {
		if IsSupportedKey(k) {
			t.Errorf("IsSupportedKey(%q) = true, want false", k)
		}
	}
}

func TestExportArgs(t *testing.T) {
	var calls []call
	doc := "<plist><dict/></plist>"
	r := &execRunner{exec: stubExec(&calls, doc, "", nil)}

	out, err := r.Export(context.Background(), ScopeCurrentHost, "com.apple.controlcenter")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if string(out) != doc {
		t.Errorf("Export = %q, want the raw output %q", out, doc)
	}
	want := []string{"-currentHost", "export", "com.apple.controlcenter", "-"}
	if len(calls) != 1 || calls[0].name != Binary || !slices.Equal(calls[0].args, want) {
		t.Errorf("calls = %+v, want one %s call with %q", calls, Binary, want)
	}
}

func TestWriteArgs(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		v     plistvalue.Value
		want  []string
	}{
		{"bool", ScopeUser, plistvalue.Boolean(true),
			[]string{"write", "com.apple.dock", "autohide", "<true/>"}},
		{"string that looks like a flag", ScopeUser, plistvalue.String("-bool"),
			[]string{"write", "com.apple.dock", "autohide", "<string>-bool</string>"}},
		{"nested", ScopeCurrentHost, plistvalue.Dict(map[string]plistvalue.Value{"a": plistvalue.Array(plistvalue.Integer(1))}),
			[]string{"-currentHost", "write", "com.apple.dock", "autohide", "<dict><key>a</key><array><integer>1</integer></array></dict>"}},
		{"system date", ScopeSystem, plistvalue.Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)),
			[]string{"write", "/Library/Preferences/com.apple.dock", "autohide", "<date>2024-01-02T03:04:05Z</date>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			r := &execRunner{exec: stubExec(&calls, "", "", nil)}
			if err := r.Write(context.Background(), tt.scope, "com.apple.dock", "autohide", tt.v); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if len(calls) != 1 || !slices.Equal(calls[0].args, tt.want) {
				t.Errorf("calls = %+v, want args %q", calls, tt.want)
			}
		})
	}
}

func TestWriteDeleteRejectBadInput(t *testing.T) {
	var calls []call
	r := &execRunner{exec: stubExec(&calls, "", "", nil)}
	ctx := context.Background()

	checks := map[string]error{
		"write bad domain": r.Write(ctx, ScopeUser, "/etc/x", "k", plistvalue.Boolean(true)),
		"write bad key":    r.Write(ctx, ScopeUser, "com.apple.dock", "-k", plistvalue.Boolean(true)),
		"write bad value":  r.Write(ctx, ScopeUser, "com.apple.dock", "k", plistvalue.String("\x01")),
		"delete bad key":   r.Delete(ctx, ScopeUser, "com.apple.dock", ""),
		"delete bad scope": r.Delete(ctx, Scope("x"), "com.apple.dock", "k"),
	}
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}
	if _, err := r.Export(ctx, ScopeUser, ".GlobalPreferences_m"); err == nil {
		t.Error("export of unsupported domain: want error, got nil")
	}
	if len(calls) != 0 {
		t.Errorf("ran defaults %d times for rejected input, want 0", len(calls))
	}
}

func TestDeleteArgsAndNotFound(t *testing.T) {
	var calls []call
	r := &execRunner{exec: stubExec(&calls, "", "", nil)}
	if err := r.Delete(context.Background(), ScopeSystem, "NSGlobalDomain", "AppleLocale"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	want := []string{"delete", "/Library/Preferences/.GlobalPreferences", "AppleLocale"}
	if !slices.Equal(calls[0].args, want) {
		t.Errorf("args = %q, want %q", calls[0].args, want)
	}

	for _, stderr := range []string{
		"Error: Could not find key 'k' in domain 'tf-acc.x'.\nDefaults have not been changed.",
		"Error: Domain 'tf-acc.x' not found.\nDefaults have not been changed.",
	} {
		r := &execRunner{exec: stubExec(&calls, "", stderr, &exec.ExitError{})}
		err := r.Delete(context.Background(), ScopeUser, "tf-acc.x", "k")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("stderr %q: error %v doesn't wrap ErrNotFound", stderr, err)
		}
		if errors.Is(err, ErrPermission) {
			t.Errorf("stderr %q: error %v wrongly wraps ErrPermission", stderr, err)
		}
	}
}

func TestErrorClassification(t *testing.T) {
	var calls []call
	r := &execRunner{exec: stubExec(&calls, "", "Could not write domain: Operation not permitted", &exec.ExitError{})}
	err := r.Write(context.Background(), ScopeUser, "com.apple.Safari", "k", plistvalue.Boolean(true))
	if !errors.Is(err, ErrPermission) {
		t.Errorf("error %v doesn't wrap ErrPermission", err)
	}
	if !strings.Contains(err.Error(), "com.apple.Safari") || !strings.Contains(err.Error(), " k ") {
		t.Errorf("Error() = %q, want it to name the domain and key", err.Error())
	}

	r = &execRunner{exec: stubExec(&calls, "", "something else", &exec.ExitError{})}
	err = r.Write(context.Background(), ScopeUser, "com.apple.dock", "k", plistvalue.Boolean(true))
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPermission) {
		t.Errorf("unclassified failure %v matched a class", err)
	}
}

func TestWritesSerialisedPerDomain(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	release := make(chan struct{})
	r := &execRunner{exec: func(context.Context, string, ...string) ([]byte, []byte, error) {
		n := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		return nil, nil, nil
	}}

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := r.Write(context.Background(), ScopeUser, "com.apple.dock", "k", plistvalue.Boolean(true)); err != nil {
				t.Error(err)
			}
		})
	}
	for range 4 {
		release <- struct{}{}
	}
	wg.Wait()
	if got := maxInFlight.Load(); got != 1 {
		t.Errorf("%d writes to one domain ran at once, want 1", got)
	}
}

func TestRemoveScratchDomainRefusesRealDomains(t *testing.T) {
	for _, d := range []string{"com.apple.dock", "NSGlobalDomain", "tf-acc", "xtf-acc.thing", "tf-acc./x"} {
		if err := RemoveScratchDomain(context.Background(), ScopeUser, d); err == nil {
			t.Errorf("RemoveScratchDomain(%q) succeeded, want refusal", d)
		}
	}
}
