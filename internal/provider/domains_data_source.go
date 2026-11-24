package provider

import (
	"context"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*domainsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*domainsDataSource)(nil)
)

// NewDomainsDataSource returns the macosdefaults_domains data source.
func NewDomainsDataSource() datasource.DataSource {
	return &domainsDataSource{}
}

type domainsDataSource struct {
	data *providerutil.ProviderData
}

type domainsDataSourceModel struct {
	Scope   types.String `tfsdk:"scope"`
	Domains types.List   `tfsdk:"domains"`
}

func (d *domainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domains"
}

func (d *domainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the preference domains in a scope that the provider can manage, sorted by name.\n\n" +
			"For `user` and `current_host` this is what `defaults domains` reports. For `system` it is the `*.plist` files in " +
			"`/Library/Preferences`. `NSGlobalDomain` is always included. Names the provider doesn't support are left out: " +
			"anything starting with `.` or `-`, containing `/` or `..`, or using characters other than letters, digits, " +
			"`.`, `_`, `-` and spaces.",
		Attributes: map[string]schema.Attribute{
			providerutil.ScopeAttributeName: providerutil.DataSourceScopeAttribute(),
			"domains": schema.ListAttribute{
				MarkdownDescription: "Domain names, sorted.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *domainsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = providerutil.DataSourceData(req, resp)
}

func (d *domainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model domainsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.data == nil {
		resp.Diagnostics.AddError("Provider not configured",
			"macosdefaults_domains was read before the provider was configured. This is a provider bug; please report it.")
		return
	}

	scope := providerutil.ScopeOrDefault(model.Scope, d.data.DefaultScope)
	resp.Diagnostics.Append(providerutil.CheckScope(scope, d.data.IsRoot)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.data.Runner.Domains(ctx, scope)
	if err != nil {
		resp.Diagnostics.AddError("Listing preference domains",
			"Couldn't list "+string(scope)+" preference domains: "+err.Error())
		return
	}

	domains, diags := types.ListValueFrom(ctx, types.StringType, defaults.SupportedDomains(raw))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	model.Scope = types.StringValue(string(scope))
	model.Domains = domains
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
