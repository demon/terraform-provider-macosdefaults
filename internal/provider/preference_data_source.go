package provider

import (
	"context"
	"fmt"

	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*preferenceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*preferenceDataSource)(nil)
)

// NewPreferenceDataSource returns the macosdefaults_preference data source.
func NewPreferenceDataSource() datasource.DataSource {
	return &preferenceDataSource{}
}

type preferenceDataSource struct {
	data *providerutil.ProviderData
}

type preferenceDataSourceModel struct {
	Domain types.String  `tfsdk:"domain"`
	Key    types.String  `tfsdk:"key"`
	Scope  types.String  `tfsdk:"scope"`
	Value  types.Dynamic `tfsdk:"value"`
	Type   types.String  `tfsdk:"type"`
}

func (d *preferenceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_preference"
}

func (d *preferenceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one preference key without managing it. A key that doesn't exist is an error.\n\n" +
			"`value` comes back as the Terraform value you'd write for it: strings, bools and numbers as themselves, " +
			"arrays as tuples, dicts as objects, dates as RFC 3339 strings and data as base64 strings. " +
			"`type` says which property list type is stored.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: providerutil.DomainDescription,
				Required:            true,
				Validators:          []validator.String{providerutil.DomainValidator()},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: providerutil.KeyDescription,
				Required:            true,
				Validators:          []validator.String{providerutil.KeyValidator()},
			},
			providerutil.ScopeAttributeName: providerutil.DataSourceScopeAttribute(),
			"value": schema.DynamicAttribute{
				MarkdownDescription: "The stored value.",
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "The stored value's " + providerutil.TypeDescription,
				Computed:            true,
			},
		},
	}
}

func (d *preferenceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = providerutil.DataSourceData(req, resp)
}

func (d *preferenceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model preferenceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.data == nil {
		resp.Diagnostics.AddError("Provider not configured",
			"macosdefaults_preference was read before the provider was configured. This is a provider bug; please report it.")
		return
	}

	scope := providerutil.ScopeOrDefault(model.Scope, d.data.DefaultScope)
	resp.Diagnostics.Append(providerutil.CheckScope(scope, d.data.IsRoot)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domain, key := model.Domain.ValueString(), model.Key.ValueString()

	v, ok, err := readKey(ctx, d.data.Runner, scope, domain, key)
	if err != nil {
		resp.Diagnostics.Append(providerutil.RunnerError("Reading preference", scope, domain, key, err))
		return
	}
	if !ok {
		resp.Diagnostics.AddAttributeError(path.Root("key"), "Preference not found",
			fmt.Sprintf("Key %q doesn't exist in domain %s (scope %s). Check the spelling, or use "+
				"`defaults read %s` to see which keys exist.", key, domain, scope, domain))
		return
	}

	tv, err := plistvalue.ToTerraform(v)
	if err != nil {
		resp.Diagnostics.AddError("Converting preference",
			fmt.Sprintf("Key %q in %s (scope %s) can't be represented in Terraform: %s", key, domain, scope, err))
		return
	}

	model.Scope = types.StringValue(string(scope))
	model.Value = tv
	model.Type = types.StringValue(string(v.Kind))
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
