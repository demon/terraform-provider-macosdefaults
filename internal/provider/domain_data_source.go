package provider

import (
	"context"
	"fmt"

	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*domainDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*domainDataSource)(nil)
)

// NewDomainDataSource returns the macosdefaults_domain data source.
func NewDomainDataSource() datasource.DataSource {
	return &domainDataSource{}
}

type domainDataSource struct {
	data *providerutil.ProviderData
}

type domainDataSourceModel struct {
	Domain types.String  `tfsdk:"domain"`
	Scope  types.String  `tfsdk:"scope"`
	Values types.Dynamic `tfsdk:"values"`
	Types  types.Map     `tfsdk:"types"`
}

func (d *domainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (d *domainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads every key in a preference domain as native Terraform values, without managing them.\n\n" +
			"`values` is an object with one attribute per key, converted the same way as `macosdefaults_preference`: " +
			"strings, bools and numbers as themselves, arrays as tuples, dicts as objects, dates as RFC 3339 strings and " +
			"data as base64 strings. `types` gives each key's property list type, which tells a date from a string or " +
			"a float from an integer.\n\n" +
			"A domain that doesn't exist reads as empty, because `defaults` can't tell it apart from a domain with no keys. " +
			"For the raw XML instead, use `macosdefaults_export`.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: providerutil.DomainDescription,
				Required:            true,
				Validators:          []validator.String{providerutil.DomainValidator()},
			},
			providerutil.ScopeAttributeName: providerutil.DataSourceScopeAttribute(),
			"values": schema.DynamicAttribute{
				MarkdownDescription: "Every key in the domain, as an object. Look keys up with `values[\"name\"]`, " +
					"or `values.name` for keys that are valid identifiers.",
				Computed: true,
			},
			"types": schema.MapAttribute{
				MarkdownDescription: "Each key's " + providerutil.TypeDescription,
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *domainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = providerutil.DataSourceData(req, resp)
}

func (d *domainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model domainDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.data == nil {
		resp.Diagnostics.AddError("Provider not configured",
			"macosdefaults_domain was read before the provider was configured. This is a provider bug; please report it.")
		return
	}

	scope := providerutil.ScopeOrDefault(model.Scope, d.data.DefaultScope)
	resp.Diagnostics.Append(providerutil.CheckScope(scope, d.data.IsRoot)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domain := model.Domain.ValueString()

	doc, err := d.data.Runner.Export(ctx, scope, domain)
	if err != nil {
		resp.Diagnostics.Append(providerutil.RunnerError("Reading preference domain", scope, domain, "", err))
		return
	}
	keys, err := plistvalue.DecodeDomain(doc)
	if err != nil {
		resp.Diagnostics.AddError("Reading preference domain",
			fmt.Sprintf("Couldn't parse `defaults export %s` output (scope %s): %s", domain, scope, err))
		return
	}

	values, err := plistvalue.ToTerraform(plistvalue.Dict(keys))
	if err != nil {
		resp.Diagnostics.AddError("Converting preference domain",
			fmt.Sprintf("Domain %s (scope %s) has a value Terraform can't represent: %s", domain, scope, err))
		return
	}

	kinds := make(map[string]string, len(keys))
	for k, v := range keys {
		kinds[k] = string(v.Kind)
	}
	typesMap, diags := types.MapValueFrom(ctx, types.StringType, kinds)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	model.Scope = types.StringValue(string(scope))
	model.Values = values
	model.Types = typesMap
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
