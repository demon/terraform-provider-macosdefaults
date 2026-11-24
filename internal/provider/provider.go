package provider

import (
	"context"
	"os"
	"runtime"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*macosdefaultsProvider)(nil)

type macosdefaultsProvider struct {
	version string

	// goos, geteuid and newRunner are indirections over the runtime so unit
	// tests can exercise the platform and root checks on any OS, and swap the
	// real defaults binary for an in-memory fake.
	goos      string
	geteuid   func() int
	newRunner func() defaults.Runner
}

type providerModel struct {
	Scope types.String `tfsdk:"scope"`
}

func (p *macosdefaultsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "macosdefaults"
	resp.Version = p.version
}

func (p *macosdefaultsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages macOS preferences through the `defaults` tool on the machine running Terraform. " +
			"It only works on macOS, and needs no credentials.\n\n" +
			"`scope` sets the default for every resource and data source; each can override it. " +
			"Which scopes a run can use depends on who runs it: `system` needs root, and the others need a normal user.",
		Attributes: map[string]schema.Attribute{
			providerutil.ScopeAttributeName: providerutil.ProviderScopeAttribute(),
		},
	}
}

func (p *macosdefaultsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	if p.goos != "darwin" {
		resp.Diagnostics.AddError(
			"Unsupported operating system",
			"The macosdefaults provider manages preferences through /usr/bin/defaults and only runs on macOS (darwin); "+
				"this machine reports "+p.goos+".",
		)
		return
	}

	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Scope.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root(providerutil.ScopeAttributeName), "Provider scope not known",
			"The provider's `scope` depends on a value that is only known after apply. Use a literal or a variable instead.")
		return
	}

	// The scope/root rule is enforced per resource and data source, on the
	// effective scope. Checking the provider default here too would reject
	// valid configs where every resource overrides it.
	data := &providerutil.ProviderData{
		Runner:       p.newRunner(),
		IsRoot:       p.geteuid() == 0,
		DefaultScope: providerutil.ScopeOrDefault(config.Scope, defaults.ScopeUser),
	}
	resp.DataSourceData = data
	resp.ResourceData = data
}

func (p *macosdefaultsProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPreferenceResource,
	}
}

func (p *macosdefaultsProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDomainDataSource,
		NewDomainsDataSource,
		NewExportDataSource,
		NewPreferenceDataSource,
	}
}

// New returns a constructor for the provider, as providerserver expects.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &macosdefaultsProvider{
			version:   version,
			goos:      runtime.GOOS,
			geteuid:   os.Geteuid,
			newRunner: defaults.New,
		}
	}
}
