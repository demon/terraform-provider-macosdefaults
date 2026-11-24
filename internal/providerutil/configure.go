package providerutil

import (
	"fmt"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// ProviderData is what the provider's Configure hands to every resource and
// data source.
type ProviderData struct {
	// Runner executes defaults(1). It is the only route to the preferences
	// system.
	Runner defaults.Runner

	// IsRoot reports whether Terraform is running with euid 0. Resources and
	// data sources use it to enforce the scope/root rule at plan time.
	IsRoot bool

	// DefaultScope is the provider's `scope`: what resources and data sources
	// use when they don't set their own.
	DefaultScope defaults.Scope
}

// ResourceData extracts ProviderData in a resource's Configure. It returns nil
// (with no diagnostic) before the provider has been configured, which the
// framework does during validation.
func ResourceData(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *ProviderData {
	return fromProviderData(req.ProviderData, &resp.Diagnostics)
}

// DataSourceData extracts ProviderData in a data source's Configure. See
// ResourceData.
func DataSourceData(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *ProviderData {
	return fromProviderData(req.ProviderData, &resp.Diagnostics)
}

func fromProviderData(providerData any, diags *diag.Diagnostics) *ProviderData {
	if providerData == nil {
		return nil
	}
	d, ok := providerData.(*ProviderData)
	if !ok {
		diags.AddError(
			"Unexpected provider data",
			fmt.Sprintf("got %T, want *providerutil.ProviderData. This is a provider bug; please report it.", providerData),
		)
		return nil
	}
	return d
}
