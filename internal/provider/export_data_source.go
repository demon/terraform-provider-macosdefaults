package provider

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*exportDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*exportDataSource)(nil)
)

// NewExportDataSource returns the macosdefaults_export data source.
func NewExportDataSource() datasource.DataSource {
	return &exportDataSource{}
}

type exportDataSource struct {
	data *providerutil.ProviderData
}

type exportDataSourceModel struct {
	Domain types.String `tfsdk:"domain"`
	Scope  types.String `tfsdk:"scope"`
	Plist  types.String `tfsdk:"plist"`
}

func (d *exportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_export"
}

func (d *exportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns a preference domain as the XML property list `defaults export <domain> -` prints, " +
			"unparsed and unchanged: for backups, diffing, or writing to a file with `local_file`.\n\n" +
			"A domain that doesn't exist exports as an empty `<dict/>` document, because `defaults` can't tell it " +
			"apart from a domain with no keys. To use the values in Terraform, read the domain with " +
			"`macosdefaults_domain` instead.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: providerutil.DomainDescription,
				Required:            true,
				Validators:          []validator.String{providerutil.DomainValidator()},
			},
			providerutil.ScopeAttributeName: providerutil.DataSourceScopeAttribute(),
			"plist": schema.StringAttribute{
				MarkdownDescription: "The XML property list document, exactly as `defaults export` printed it.",
				Computed:            true,
			},
		},
	}
}

func (d *exportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = providerutil.DataSourceData(req, resp)
}

func (d *exportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model exportDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.data == nil {
		resp.Diagnostics.AddError("Provider not configured",
			"macosdefaults_export was read before the provider was configured. This is a provider bug; please report it.")
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
		resp.Diagnostics.Append(providerutil.RunnerError("Exporting preference domain", scope, domain, "", err))
		return
	}
	// Terraform strings must be UTF-8. defaults export always writes UTF-8
	// XML, so anything else means its output wasn't what we expect.
	if !utf8.Valid(doc) {
		resp.Diagnostics.AddError("Exporting preference domain",
			fmt.Sprintf("`defaults export %s` (scope %s) printed something that isn't valid UTF-8, so it can't be "+
				"returned as a string. This is unexpected; please report it.", domain, scope))
		return
	}

	model.Scope = types.StringValue(string(scope))
	model.Plist = types.StringValue(string(doc))
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
