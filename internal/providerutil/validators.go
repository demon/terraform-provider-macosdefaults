package providerutil

import (
	"context"
	"fmt"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// DomainDescription documents the `domain` attribute everywhere it appears.
const DomainDescription = "Preference domain, usually an app's bundle identifier (`com.apple.dock`), or `NSGlobalDomain` " +
	"for settings shared by every app. Names must start with a letter or digit and contain only letters, digits, " +
	"`.`, `_`, `-` and spaces; aliases such as `-g` and `.GlobalPreferences`, and file paths, aren't accepted."

// KeyDescription documents the `key` attribute everywhere it appears.
const KeyDescription = "Preference key within the domain. Any non-empty name works, except one starting with `-` " +
	"or containing control characters."

// TypeDescription documents the `type` attribute.
const TypeDescription = "Property list type: one of `string`, `integer`, `float`, `boolean`, `date`, `data`, `array` or `dict`."

// DomainValidator accepts only names defaults.IsSupportedDomain accepts.
func DomainValidator() validator.String {
	return funcValidator{
		desc: "must be a supported preference domain name",
		ok:   defaults.IsSupportedDomain,
		msg: func(v string) string {
			return fmt.Sprintf("%q isn't a supported domain name. Use the app's bundle identifier (e.g. `com.apple.dock`) "+
				"or `NSGlobalDomain`; aliases like `-g` and `.GlobalPreferences`, and paths, aren't accepted.", v)
		},
	}
}

// KeyValidator accepts only keys defaults.IsSupportedKey accepts.
func KeyValidator() validator.String {
	return funcValidator{
		desc: "must be a non-empty key that doesn't start with `-`",
		ok:   defaults.IsSupportedKey,
		msg: func(v string) string {
			return fmt.Sprintf("%q isn't a supported key: keys can't be empty, start with `-`, or contain control characters.", v)
		},
	}
}

// TypeValidator accepts only plistvalue.Kinds.
func TypeValidator() validator.String {
	names := make([]string, 0, len(plistvalue.Kinds()))
	for _, k := range plistvalue.Kinds() {
		names = append(names, string(k))
	}
	return stringvalidator.OneOf(names...)
}

type funcValidator struct {
	desc string
	ok   func(string) bool
	msg  func(string) string
}

func (v funcValidator) Description(context.Context) string         { return v.desc }
func (v funcValidator) MarkdownDescription(context.Context) string { return v.desc }

func (v funcValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if s := req.ConfigValue.ValueString(); !v.ok(s) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", v.msg(s))
	}
}
