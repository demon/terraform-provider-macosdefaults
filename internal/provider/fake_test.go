package provider

import (
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// fakeFactories serves the provider with fake (normally a
// *defaultstest.Fake) in place of the real defaults binary, as if on macOS
// with the given euid. The same fake is shared by every provider instance the
// test harness starts, so state carries across steps. Tests using it run
// Terraform but never touch real preferences, so they run on any OS as part
// of `make test`.
func fakeFactories(fake defaults.Runner, euid int) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"macosdefaults": func() (tfprotov6.ProviderServer, error) {
			p := &macosdefaultsProvider{
				version:   "test",
				goos:      "darwin",
				geteuid:   func() int { return euid },
				newRunner: func() defaults.Runner { return fake },
			}
			return providerserver.NewProtocol6WithError(p)()
		},
	}
}
