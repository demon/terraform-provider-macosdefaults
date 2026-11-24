package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = (*preferenceResource)(nil)
	_ resource.ResourceWithConfigure      = (*preferenceResource)(nil)
	_ resource.ResourceWithValidateConfig = (*preferenceResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*preferenceResource)(nil)
	_ resource.ResourceWithImportState    = (*preferenceResource)(nil)
)

// NewPreferenceResource returns the macosdefaults_preference resource.
func NewPreferenceResource() resource.Resource {
	return &preferenceResource{}
}

type preferenceResource struct {
	data *providerutil.ProviderData
}

type preferenceResourceModel struct {
	ID     types.String  `tfsdk:"id"`
	Domain types.String  `tfsdk:"domain"`
	Key    types.String  `tfsdk:"key"`
	Scope  types.String  `tfsdk:"scope"`
	Value  types.Dynamic `tfsdk:"value"`
	Type   types.String  `tfsdk:"type"`
}

func (r *preferenceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_preference"
}

func (r *preferenceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one preference key in one domain, as `defaults write` would set it.\n\n" +
			"The property list type is inferred from `value`: strings, bools, whole numbers (`integer`), fractional " +
			"numbers (`float`), lists (`array`) and maps or objects (`dict`), recursively. Set `type` to override the " +
			"inferred type of the top-level value: `float` for a whole number an app expects as a real, `date` for an " +
			"RFC 3339 string, or `data` for a base64 string.\n\n" +
			"Changes made outside Terraform (in System Settings, or by the app) show up as drift. A boolean stored by " +
			"the app as the integer 1 or 0 counts as unchanged.\n\n" +
			"Destroying the resource deletes the key, so the app goes back to its built-in default. The rest of the " +
			"domain is never touched. The provider doesn't restart apps; many only read preferences at launch.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`<scope>/<domain>:<key>`, naming the effective scope, e.g. " +
					"`user/com.apple.dock:autohide`. Usable as an import ID.",
				Computed: true,
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: providerutil.DomainDescription + " Changing it replaces the resource.",
				Required:            true,
				Validators:          []validator.String{providerutil.DomainValidator()},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: providerutil.KeyDescription + " Changing it replaces the resource.",
				Required:            true,
				Validators:          []validator.String{providerutil.KeyValidator()},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			providerutil.ScopeAttributeName: providerutil.ResourceScopeAttribute(),
			"value": schema.DynamicAttribute{
				MarkdownDescription: "The value to store: a string, bool, number, list or map, nested as deeply as needed. " +
					"Nulls can't be stored. Dates and data need `type`.",
				Required: true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: providerutil.TypeDescription + " Inferred from `value` when unset; when set, it must " +
					"fit `value` (a number for `integer` or `float`, an RFC 3339 string with whole seconds for `date`, " +
					"a base64 string for `data`).",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{providerutil.TypeValidator()},
			},
		},
	}
}

func (r *preferenceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = providerutil.ResourceData(req, resp)
}

// ValidateConfig checks value against type (or inference) at plan time, so a
// value that can't be stored fails before anything is written.
func (r *preferenceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config preferenceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Value.IsUnknown() || config.Type.IsUnknown() {
		return
	}
	override := plistvalue.Kind(config.Type.ValueString())
	if override != "" && !override.Valid() {
		return // the type validator reports this
	}
	if _, err := plistvalue.FromTerraform(config.Value, override); err != nil && !errors.Is(err, plistvalue.ErrUnknown) {
		resp.Diagnostics.AddAttributeError(path.Root("value"), "Invalid preference value", err.Error())
	}
}

// ModifyPlan resolves the effective scope (the resource's own, or the
// provider's default), requires replacement when it changes, enforces the
// scope/root rule (including for destroy), and fills in the computed id and
// inferred type so the plan shows them.
func (r *preferenceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.data == nil {
		return
	}

	if req.Plan.Raw.IsNull() {
		var state preferenceResourceModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.Append(providerutil.CheckScope(providerutil.ScopeOrDefault(state.Scope, r.data.DefaultScope), r.data.IsRoot)...)
		}
		return
	}

	var plan, config preferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state *preferenceResourceModel
	if !req.State.Raw.IsNull() {
		state = &preferenceResourceModel{}
		resp.Diagnostics.Append(req.State.Get(ctx, state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Scope and id.
	planID := types.StringUnknown()
	if config.Scope.IsUnknown() {
		plan.Scope = types.StringUnknown()
		if state != nil {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root(providerutil.ScopeAttributeName))
		}
	} else {
		scope := providerutil.ScopeOrDefault(config.Scope, r.data.DefaultScope)
		resp.Diagnostics.Append(providerutil.CheckScope(scope, r.data.IsRoot)...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Scope = types.StringValue(string(scope))
		if state != nil && state.Scope.ValueString() != string(scope) {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root(providerutil.ScopeAttributeName))
		}
		if !plan.Domain.IsUnknown() && !plan.Key.IsUnknown() {
			planID = types.StringValue(preferenceID(scope, plan.Domain.ValueString(), plan.Key.ValueString()))
		}
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(providerutil.ScopeAttributeName), plan.Scope)...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), planID)...)

	// Type.
	if !config.Type.IsNull() {
		return // configured, so the plan already has it
	}
	planType := types.StringUnknown()
	v, err := plistvalue.FromTerraform(plan.Value, "")
	switch {
	case errors.Is(err, plistvalue.ErrUnknown):
	case err != nil:
		return // ValidateConfig reports this
	default:
		planType = types.StringValue(string(v.Kind))
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("type"), planType)...)
}

func (r *preferenceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan preferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.write(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *preferenceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan preferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.write(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// write stores plan's value, reads it back to confirm it landed, and fills in
// the computed attributes.
func (r *preferenceResource) write(ctx context.Context, plan *preferenceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.data == nil {
		diags.AddError("Provider not configured", "macosdefaults_preference was applied before the provider was configured. This is a provider bug; please report it.")
		return diags
	}
	scope := providerutil.ScopeOrDefault(plan.Scope, r.data.DefaultScope)
	if diags = providerutil.CheckScope(scope, r.data.IsRoot); diags.HasError() {
		return diags
	}
	domain, key := plan.Domain.ValueString(), plan.Key.ValueString()

	override := plistvalue.Kind("")
	if !plan.Type.IsUnknown() && !plan.Type.IsNull() {
		override = plistvalue.Kind(plan.Type.ValueString())
	}
	v, err := plistvalue.FromTerraform(plan.Value, override)
	if err != nil {
		diags.AddAttributeError(path.Root("value"), "Invalid preference value", err.Error())
		return diags
	}

	if err := r.data.Runner.Write(ctx, scope, domain, key, v); err != nil {
		diags.Append(providerutil.RunnerError("Writing preference", scope, domain, key, err))
		return diags
	}

	got, ok, err := readKey(ctx, r.data.Runner, scope, domain, key)
	if err != nil {
		diags.Append(providerutil.RunnerError("Reading back preference", scope, domain, key, err))
		return diags
	}
	if !ok || !plistvalue.Equal(got, v) {
		diags.AddError("Preference didn't take effect",
			fmt.Sprintf("Wrote key %q in %s (scope %s), but reading it back gave a different value. "+
				"Something (often the app itself, if it's running) may be overwriting it.", key, domain, scope))
		return diags
	}

	plan.ID = types.StringValue(preferenceID(scope, domain, key))
	plan.Scope = types.StringValue(string(scope))
	plan.Type = types.StringValue(string(v.Kind))
	return diags
}

func (r *preferenceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state preferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.data == nil {
		resp.Diagnostics.AddError("Provider not configured", "macosdefaults_preference was read before the provider was configured. This is a provider bug; please report it.")
		return
	}
	scope := providerutil.ScopeOrDefault(state.Scope, r.data.DefaultScope)
	resp.Diagnostics.Append(providerutil.CheckScope(scope, r.data.IsRoot)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domain, key := state.Domain.ValueString(), state.Key.ValueString()

	disk, ok, err := readKey(ctx, r.data.Runner, scope, domain, key)
	if err != nil {
		resp.Diagnostics.Append(providerutil.RunnerError("Reading preference", scope, domain, key, err))
		return
	}
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(preferenceID(scope, domain, key))
	state.Scope = types.StringValue(string(scope))

	// Keep the prior state's value when it means the same thing as what's on
	// disk, so equivalent spellings (an object vs a map, a boolean stored as
	// 1) don't show up as drift.
	if !state.Value.IsNull() && !state.Value.IsUnknown() {
		prior, err := plistvalue.FromTerraform(state.Value, plistvalue.Kind(state.Type.ValueString()))
		if err == nil && plistvalue.Equal(prior, disk) {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	tv, err := plistvalue.ToTerraform(disk)
	if err != nil {
		resp.Diagnostics.AddError("Converting preference",
			fmt.Sprintf("Key %q in %s (scope %s) can't be represented in Terraform: %s", key, domain, scope, err))
		return
	}
	state.Value = tv
	state.Type = types.StringValue(string(disk.Kind))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *preferenceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state preferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.data == nil {
		resp.Diagnostics.AddError("Provider not configured", "macosdefaults_preference was destroyed before the provider was configured. This is a provider bug; please report it.")
		return
	}
	scope := providerutil.ScopeOrDefault(state.Scope, r.data.DefaultScope)
	resp.Diagnostics.Append(providerutil.CheckScope(scope, r.data.IsRoot)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domain, key := state.Domain.ValueString(), state.Key.ValueString()

	if err := r.data.Runner.Delete(ctx, scope, domain, key); err != nil && !errors.Is(err, defaults.ErrNotFound) {
		resp.Diagnostics.Append(providerutil.RunnerError("Deleting preference", scope, domain, key, err))
	}
}

func (r *preferenceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scope, domain, key, err := parsePreferenceID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	if scope == "" {
		if r.data == nil {
			resp.Diagnostics.AddError("Provider not configured", "macosdefaults_preference was imported before the provider was configured. This is a provider bug; please report it.")
			return
		}
		scope = r.data.DefaultScope
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), preferenceID(scope, domain, key))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), key)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope"), string(scope))...)
}

// parsePreferenceID splits an import ID: an optional `user/`, `current_host/`
// or `system/` prefix, then the domain, then the key after the first colon
// (domains can't contain colons; keys can). With no prefix, the returned scope
// is empty, meaning the provider's default.
func parsePreferenceID(id string) (defaults.Scope, string, string, error) {
	const format = "import IDs look like `<domain>:<key>` (the provider's default scope) or " +
		"`<scope>/<domain>:<key>` with scope `user`, `current_host` or `system`"

	scope, rest := defaults.Scope(""), id
	for _, s := range defaults.Scopes() {
		if after, ok := strings.CutPrefix(id, string(s)+"/"); ok {
			scope, rest = s, after
			break
		}
	}
	domain, key, ok := strings.Cut(rest, ":")
	if !ok {
		return "", "", "", fmt.Errorf("%q has no `:` between domain and key; %s", id, format)
	}
	if !defaults.IsSupportedDomain(domain) {
		return "", "", "", fmt.Errorf("%q: %q isn't a supported domain name; %s", id, domain, format)
	}
	if !defaults.IsSupportedKey(key) {
		return "", "", "", fmt.Errorf("%q: %q isn't a supported key; %s", id, key, format)
	}
	return scope, domain, key, nil
}
