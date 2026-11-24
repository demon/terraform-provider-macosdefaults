package providerutil

import (
	"fmt"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	rsschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ScopeAttributeName is the attribute the provider, every resource and every
// data source use for the preference scope.
const ScopeAttributeName = "scope"

const scopeValues = "`user` for the current user's preferences, " +
	"`current_host` for the current user's preferences on this Mac only (`defaults -currentHost`), " +
	"or `system` for machine-wide preferences in `/Library/Preferences`. " +
	"`system` requires running Terraform as root, and the other scopes require *not* running as root."

const overrideDescription = "Which preferences to use: " + scopeValues +
	" Defaults to the provider's `scope`."

// ProviderScopeAttribute is the schema for the provider's default `scope`.
func ProviderScopeAttribute() pschema.StringAttribute {
	return pschema.StringAttribute{
		MarkdownDescription: "The default scope for every resource and data source that doesn't set its own: " +
			scopeValues + " Defaults to `user`. Set it to `system` in a configuration you apply with `sudo`.",
		Optional:   true,
		Validators: []validator.String{ScopeValidator()},
	}
}

// DataSourceScopeAttribute is the schema for a data source's `scope`. It is
// Optional + Computed so state always records the effective scope.
func DataSourceScopeAttribute() dsschema.StringAttribute {
	return dsschema.StringAttribute{
		MarkdownDescription: overrideDescription,
		Optional:            true,
		Computed:            true,
		Validators:          []validator.String{ScopeValidator()},
	}
}

// ResourceScopeAttribute is the schema for a resource's `scope`. It is
// Optional + Computed: the resource's ModifyPlan fills in the effective scope
// (its own, or the provider's default) and requires replacement when that
// changes. The usual RequiresReplace plan modifier can't be used, because it
// would see the computed value as unknown and force replacement every time.
func ResourceScopeAttribute() rsschema.StringAttribute {
	return rsschema.StringAttribute{
		MarkdownDescription: overrideDescription + " Changing the effective scope (here or on the provider) " +
			"replaces the resource.",
		Optional:   true,
		Computed:   true,
		Validators: []validator.String{ScopeValidator()},
	}
}

// ScopeValidator accepts only the scopes in defaults.Scopes.
func ScopeValidator() validator.String {
	names := make([]string, 0, len(defaults.Scopes()))
	for _, s := range defaults.Scopes() {
		names = append(names, string(s))
	}
	return stringvalidator.OneOf(names...)
}

// ScopeOrDefault returns the configured scope, or def when it is null, empty
// or unknown. def is normally ProviderData.DefaultScope.
func ScopeOrDefault(v types.String, def defaults.Scope) defaults.Scope {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return def
	}
	return defaults.Scope(v.ValueString())
}

// CheckScope enforces the scope/root rule: system scope only as root, every
// other scope only as non-root. It is a hard error, not a warning: running as
// root, the user scopes would silently act on root's own preferences.
func CheckScope(scope defaults.Scope, isRoot bool) diag.Diagnostics {
	var diags diag.Diagnostics
	attr := path.Root(ScopeAttributeName)

	switch {
	case scope.RequiresRoot() && !isRoot:
		diags.AddAttributeError(attr,
			"scope requires root",
			fmt.Sprintf("scope = %q reads and writes /Library/Preferences, which needs root. "+
				"Apply this configuration with `sudo terraform`, keeping system-wide settings in their own root module.", scope),
		)
	case !scope.RequiresRoot() && isRoot:
		diags.AddAttributeError(attr,
			"scope can't be used as root",
			fmt.Sprintf("Terraform is running as root, so scope = %q would act on root's own preferences instead of yours. "+
				"If this configuration is meant for system-wide settings, set `scope = \"system\"` in the provider block. "+
				"Otherwise run it as your normal user, and keep `system` scope settings in a separate root module applied with `sudo`.", scope),
		)
	}
	return diags
}
