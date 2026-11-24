package providerutil

import (
	"testing"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCheckScope(t *testing.T) {
	tests := []struct {
		scope   defaults.Scope
		isRoot  bool
		wantErr bool
	}{
		{defaults.ScopeUser, false, false},
		{defaults.ScopeCurrentHost, false, false},
		{defaults.ScopeSystem, true, false},
		{defaults.ScopeUser, true, true},
		{defaults.ScopeCurrentHost, true, true},
		{defaults.ScopeSystem, false, true},
	}
	for _, tt := range tests {
		diags := CheckScope(tt.scope, tt.isRoot)
		if got := diags.HasError(); got != tt.wantErr {
			t.Errorf("CheckScope(%q, root=%v) error = %v, want %v (%v)", tt.scope, tt.isRoot, got, tt.wantErr, diags)
		}
	}
}

func TestScopeOrDefault(t *testing.T) {
	tests := []struct {
		in   types.String
		want defaults.Scope
	}{
		{types.StringNull(), defaults.ScopeUser},
		{types.StringUnknown(), defaults.ScopeUser},
		{types.StringValue(""), defaults.ScopeUser},
		{types.StringValue("system"), defaults.ScopeSystem},
		{types.StringValue("current_host"), defaults.ScopeCurrentHost},
	}
	for _, tt := range tests {
		if got := ScopeOrDefault(tt.in, defaults.ScopeUser); got != tt.want {
			t.Errorf("ScopeOrDefault(%v, user) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := ScopeOrDefault(types.StringNull(), defaults.ScopeSystem); got != defaults.ScopeSystem {
		t.Errorf("ScopeOrDefault(null, system) = %q, want the default", got)
	}
	if got := ScopeOrDefault(types.StringValue("user"), defaults.ScopeSystem); got != defaults.ScopeUser {
		t.Errorf("ScopeOrDefault(user, system) = %q, want the explicit scope", got)
	}
}
