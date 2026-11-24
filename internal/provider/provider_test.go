package provider

import (
	"context"
	"testing"

	"github.com/demon/terraform-provider-macosdefaults/internal/acctest"
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var protoV6ProviderFactories = acctest.ProtoV6ProviderFactories(New("test")())

func testProvider(goos string, euid int) *macosdefaultsProvider {
	p, ok := New("test")().(*macosdefaultsProvider)
	if !ok {
		panic("New did not return *macosdefaultsProvider")
	}
	p.goos = goos
	p.geteuid = func() int { return euid }
	return p
}

func TestProviderSchemaIsValid(t *testing.T) {
	var resp provider.SchemaResponse
	testProvider("darwin", 501).Schema(context.Background(), provider.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("ValidateImplementation: %v", diags)
	}
}

func TestProviderMetadata(t *testing.T) {
	var resp provider.MetadataResponse
	testProvider("darwin", 501).Metadata(context.Background(), provider.MetadataRequest{}, &resp)

	if resp.TypeName != "macosdefaults" {
		t.Errorf("TypeName = %q, want %q", resp.TypeName, "macosdefaults")
	}
	if resp.Version != "test" {
		t.Errorf("Version = %q, want %q", resp.Version, "test")
	}
}

// configureRequest builds a ConfigureRequest for the provider's schema with
// scope set (nil for unset, unknown for an unknown value).
func configureRequest(t *testing.T, p *macosdefaultsProvider, scope tftypes.Value) provider.ConfigureRequest {
	t.Helper()
	var schemaResp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema
	objType := s.Type().TerraformType(context.Background())
	return provider.ConfigureRequest{Config: tfsdk.Config{
		Schema: s,
		Raw:    tftypes.NewValue(objType, map[string]tftypes.Value{"scope": scope}),
	}}
}

func TestProviderConfigure(t *testing.T) {
	unset := tftypes.NewValue(tftypes.String, nil)
	tests := []struct {
		name      string
		goos      string
		euid      int
		scope     tftypes.Value
		wantErr   bool
		wantRoot  bool
		wantScope defaults.Scope
	}{
		{name: "darwin user", goos: "darwin", euid: 501, scope: unset, wantScope: defaults.ScopeUser},
		{name: "darwin root", goos: "darwin", euid: 0, scope: unset, wantRoot: true, wantScope: defaults.ScopeUser},
		{name: "explicit system", goos: "darwin", euid: 0, scope: tftypes.NewValue(tftypes.String, "system"), wantRoot: true, wantScope: defaults.ScopeSystem},
		{name: "explicit current_host", goos: "darwin", euid: 501, scope: tftypes.NewValue(tftypes.String, "current_host"), wantScope: defaults.ScopeCurrentHost},
		// The root rule is checked per resource on the effective scope, not here.
		{name: "system default as non-root", goos: "darwin", euid: 501, scope: tftypes.NewValue(tftypes.String, "system"), wantScope: defaults.ScopeSystem},
		{name: "unknown scope", goos: "darwin", euid: 501, scope: tftypes.NewValue(tftypes.String, tftypes.UnknownValue), wantErr: true},
		{name: "linux", goos: "linux", euid: 1000, scope: unset, wantErr: true},
		{name: "windows", goos: "windows", euid: -1, scope: unset, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProvider(tt.goos, tt.euid)
			var resp provider.ConfigureResponse
			p.Configure(context.Background(), configureRequest(t, p, tt.scope), &resp)

			if got := resp.Diagnostics.HasError(); got != tt.wantErr {
				t.Fatalf("HasError = %v, want %v (diagnostics: %v)", got, tt.wantErr, resp.Diagnostics)
			}
			if tt.wantErr {
				if resp.ResourceData != nil || resp.DataSourceData != nil {
					t.Fatal("provider data set despite configure error")
				}
				return
			}

			for name, raw := range map[string]any{"ResourceData": resp.ResourceData, "DataSourceData": resp.DataSourceData} {
				data, ok := raw.(*providerutil.ProviderData)
				if !ok {
					t.Fatalf("%s is %T, want *providerutil.ProviderData", name, raw)
				}
				if data.Runner == nil {
					t.Errorf("%s.Runner is nil", name)
				}
				if data.IsRoot != tt.wantRoot {
					t.Errorf("%s.IsRoot = %v, want %v", name, data.IsRoot, tt.wantRoot)
				}
				if data.DefaultScope != tt.wantScope {
					t.Errorf("%s.DefaultScope = %q, want %q", name, data.DefaultScope, tt.wantScope)
				}
			}
		})
	}
}
