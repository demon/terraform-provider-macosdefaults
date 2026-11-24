package provider

import (
	"context"
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/demon/terraform-provider-macosdefaults/internal/acctest"
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults/defaultstest"
	"github.com/demon/terraform-provider-macosdefaults/internal/providerutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// readDomains runs the data source's Read against fake with the given scope
// (nil for unset) and returns the response and the decoded state.
func readDomains(t *testing.T, fake *defaultstest.Fake, isRoot bool, scope *string) (*datasource.ReadResponse, domainsDataSourceModel) {
	t.Helper()
	return readDomainsWithDefault(t, fake, isRoot, defaults.ScopeUser, scope)
}

func readDomainsWithDefault(t *testing.T, fake *defaultstest.Fake, isRoot bool, def defaults.Scope, scope *string) (*datasource.ReadResponse, domainsDataSourceModel) {
	t.Helper()
	ctx := context.Background()

	ds := &domainsDataSource{data: &providerutil.ProviderData{Runner: fake, IsRoot: isRoot, DefaultScope: def}}

	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema
	objType := s.Type().TerraformType(ctx)

	var scopeVal tftypes.Value
	if scope == nil {
		scopeVal = tftypes.NewValue(tftypes.String, nil)
	} else {
		scopeVal = tftypes.NewValue(tftypes.String, *scope)
	}
	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
		"scope":   scopeVal,
		"domains": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(objType, nil)}}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)

	var model domainsDataSourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &model); diags.HasError() {
			t.Fatalf("decoding state: %v", diags)
		}
	}
	return resp, model
}

func stateDomains(t *testing.T, m domainsDataSourceModel) []string {
	t.Helper()
	var got []string
	if diags := m.Domains.ElementsAs(context.Background(), &got, false); diags.HasError() {
		t.Fatalf("decoding domains: %v", diags)
	}
	return got
}

func ptr(s string) *string { return &s }

func TestDomainsDataSourceRead(t *testing.T) {
	fake := &defaultstest.Fake{DomainsByScope: map[defaults.Scope][]string{
		defaults.ScopeUser:        {"com.apple.dock", ".GlobalPreferences_m", "loginwindow", "com.apple.Finder"},
		defaults.ScopeCurrentHost: {"com.apple.controlcenter"},
		defaults.ScopeSystem:      {"com.apple.loginwindow", ".GlobalPreferences"},
	}}

	tests := []struct {
		name      string
		scope     *string
		isRoot    bool
		wantScope string
		want      []string
	}{
		{
			name:      "unset scope defaults to user",
			wantScope: "user",
			want:      []string{"NSGlobalDomain", "com.apple.Finder", "com.apple.dock", "loginwindow"},
		},
		{
			name:      "current_host",
			scope:     ptr("current_host"),
			wantScope: "current_host",
			want:      []string{"NSGlobalDomain", "com.apple.controlcenter"},
		},
		{
			name:      "system as root",
			scope:     ptr("system"),
			isRoot:    true,
			wantScope: "system",
			want:      []string{"NSGlobalDomain", "com.apple.loginwindow"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, model := readDomains(t, fake, tt.isRoot, tt.scope)
			if resp.Diagnostics.HasError() {
				t.Fatalf("Read: %v", resp.Diagnostics)
			}
			if got := model.Scope.ValueString(); got != tt.wantScope {
				t.Errorf("scope = %q, want %q", got, tt.wantScope)
			}
			if got := stateDomains(t, model); !slices.Equal(got, tt.want) {
				t.Errorf("domains = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDomainsDataSourceProviderDefaultScope(t *testing.T) {
	fake := &defaultstest.Fake{DomainsByScope: map[defaults.Scope][]string{
		defaults.ScopeUser:   {"com.apple.dock"},
		defaults.ScopeSystem: {"com.apple.loginwindow"},
	}}

	resp, model := readDomainsWithDefault(t, fake, true, defaults.ScopeSystem, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	if got := model.Scope.ValueString(); got != "system" {
		t.Errorf("scope = %q, want the provider default %q", got, "system")
	}
	if got := stateDomains(t, model); !slices.Equal(got, []string{"NSGlobalDomain", "com.apple.loginwindow"}) {
		t.Errorf("domains = %q, want the system listing", got)
	}

	// An explicit scope overrides the provider default (and is still
	// checked against the root rule).
	resp, _ = readDomainsWithDefault(t, fake, true, defaults.ScopeSystem, ptr("user"))
	if !resp.Diagnostics.HasError() {
		t.Error("explicit user scope as root: want error, got none")
	}
}

func TestDomainsDataSourceScopeRootRule(t *testing.T) {
	tests := []struct {
		name   string
		scope  *string
		isRoot bool
	}{
		{"user as root", nil, true},
		{"current_host as root", ptr("current_host"), true},
		{"system as non-root", ptr("system"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &defaultstest.Fake{}
			resp, _ := readDomains(t, fake, tt.isRoot, tt.scope)
			if !resp.Diagnostics.HasError() {
				t.Fatal("Read: want scope/root error, got none")
			}
			if len(fake.DomainsCalls) != 0 {
				t.Errorf("listed domains %d times despite the scope error, want 0", len(fake.DomainsCalls))
			}
		})
	}
}

func TestDomainsDataSourceRunnerError(t *testing.T) {
	fake := &defaultstest.Fake{DomainsErr: errors.New("defaults domains failed with exit status 1: boom")}
	resp, _ := readDomains(t, fake, false, nil)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Read: want error, got none")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "user") || !strings.Contains(detail, "boom") {
		t.Errorf("error detail = %q, want it to name the scope and include the cause", detail)
	}
}

func TestDomainsDataSourceUnconfigured(t *testing.T) {
	ds := &domainsDataSource{}
	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema
	objType := s.Type().TerraformType(context.Background())

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(objType, nil)}}
	ds.Read(context.Background(), datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
		"scope":   tftypes.NewValue(tftypes.String, nil),
		"domains": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})}}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Read on an unconfigured data source: want error, got none")
	}
}

func TestDomainsDataSourceSchemaIsValid(t *testing.T) {
	var resp datasource.SchemaResponse
	NewDomainsDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("ValidateImplementation: %v", diags)
	}
}

func TestAccDomainsDataSource(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("user-scope acceptance tests can't run as root")
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Expected errors first: the post-test destroy reuses the last step's config.
			{
				Config:      `data "macosdefaults_domains" "test" { scope = "system" }`,
				ExpectError: regexp.MustCompile(`scope requires root`),
			},
			{
				Config:      `data "macosdefaults_domains" "test" { scope = "bogus" }`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
			{
				Config: `data "macosdefaults_domains" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.macosdefaults_domains.test", "scope", "user"),
					resource.TestCheckTypeSetElemAttr("data.macosdefaults_domains.test", "domains.*", "NSGlobalDomain"),
				),
			},
			{
				Config: `data "macosdefaults_domains" "test" { scope = "current_host" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.macosdefaults_domains.test", "scope", "current_host"),
					resource.TestCheckTypeSetElemAttr("data.macosdefaults_domains.test", "domains.*", "NSGlobalDomain"),
				),
			},
		},
	})
}

func TestAccSystemDomainsDataSource(t *testing.T) {
	if os.Getenv("MACOSDEFAULTS_TEST_SYSTEM") == "" {
		t.Skip("set MACOSDEFAULTS_TEST_SYSTEM and run as root to test system scope")
	}
	if os.Geteuid() != 0 {
		t.Skip("system-scope acceptance tests must run as root")
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "macosdefaults_domains" "test" {}`,
				ExpectError: regexp.MustCompile(`scope can't be used as root`),
			},
			{
				Config: `data "macosdefaults_domains" "test" { scope = "system" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.macosdefaults_domains.test", "scope", "system"),
					resource.TestCheckTypeSetElemAttr("data.macosdefaults_domains.test", "domains.*", "NSGlobalDomain"),
				),
			},
		},
	})
}
