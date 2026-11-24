// Package acctest holds shared helpers for acceptance tests.
//
// Acceptance tests run the real /usr/bin/defaults, so they only make sense on
// macOS, and they must only touch scratch domains created by ScratchDomain.
package acctest

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
)

// PreCheck skips the test when it can't run here (anywhere but macOS).
func PreCheck(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skipf("acceptance tests need macOS; this is %s", runtime.GOOS)
	}
}

// PreCheckUserScope also skips when running as root, where the user scopes
// are refused.
func PreCheckUserScope(t *testing.T) {
	t.Helper()
	PreCheck(t)
	if os.Geteuid() == 0 {
		t.Skip("user-scope acceptance tests can't run as root")
	}
}

// PreCheckCurrentHost also skips unless MACOSDEFAULTS_TEST_CURRENT_HOST is
// set, because those tests write to the real ByHost preferences.
func PreCheckCurrentHost(t *testing.T) {
	t.Helper()
	PreCheckUserScope(t)
	if os.Getenv("MACOSDEFAULTS_TEST_CURRENT_HOST") == "" {
		t.Skip("set MACOSDEFAULTS_TEST_CURRENT_HOST to test current_host scope")
	}
}

// PreCheckSystem skips unless MACOSDEFAULTS_TEST_SYSTEM is set and the test
// is running as root.
func PreCheckSystem(t *testing.T) {
	t.Helper()
	PreCheck(t)
	if os.Getenv("MACOSDEFAULTS_TEST_SYSTEM") == "" {
		t.Skip("set MACOSDEFAULTS_TEST_SYSTEM and run as root to test system scope")
	}
	if os.Geteuid() != 0 {
		t.Skip("system-scope acceptance tests must run as root")
	}
}

// ScratchDomain returns a fresh tf-acc.<random> domain name in scope and
// registers a cleanup that removes the whole domain when the test ends.
func ScratchDomain(t *testing.T, scope defaults.Scope) string {
	t.Helper()
	domain := defaults.ScratchDomainPrefix + acctest.RandString(12)
	t.Cleanup(func() {
		if err := defaults.RemoveScratchDomain(context.Background(), scope, domain); err != nil {
			t.Errorf("removing scratch domain %s: %v", domain, err)
		}
	})
	return domain
}

// ProtoV6ProviderFactories wires p into terraform-plugin-testing under the
// provider's registry name.
func ProtoV6ProviderFactories(p provider.Provider) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"macosdefaults": providerserver.NewProtocol6WithError(p),
	}
}
