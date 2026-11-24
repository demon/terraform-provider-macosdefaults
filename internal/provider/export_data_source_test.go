package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/demon/terraform-provider-macosdefaults/internal/acctest"
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults/defaultstest"
	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const exportDataAddr = "data.macosdefaults_export.test"

func exportDataConfig(domain, extra string) string {
	return fmt.Sprintf(`
data "macosdefaults_export" "test" {
  domain = %q
%s
}
`, domain, extra)
}

func exportPlist(want string) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(exportDataAddr, tfjsonpath.New("plist"), knownvalue.StringExact(want))
}

// rawExport is a runner whose Export returns fixed bytes, for output the
// in-memory fake would never produce.
type rawExport struct {
	*defaultstest.Fake
	raw []byte
}

func (r rawExport) Export(context.Context, defaults.Scope, string) ([]byte, error) { return r.raw, nil }

func TestExportDataSource(t *testing.T) {
	fake := &defaultstest.Fake{}
	fake.Set(defaults.ScopeUser, testDomain, "flag", plistvalue.Boolean(true))
	fake.Set(defaults.ScopeUser, testDomain, "name", plistvalue.String("a & <b>"))
	fake.Set(defaults.ScopeCurrentHost, testDomain, "flag", plistvalue.Boolean(false))

	export := func(scope defaults.Scope, domain string) string {
		t.Helper()
		doc, err := fake.Export(context.Background(), scope, domain)
		if err != nil {
			t.Fatal(err)
		}
		return string(doc)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				// Passed through byte for byte, entities and all.
				Config: exportDataConfig(testDomain, "") + `
output "has_name" {
  value = strcontains(data.macosdefaults_export.test.plist, "<string>a &amp; &lt;b&gt;</string>")
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					exportPlist(export(defaults.ScopeUser, testDomain)),
					statecheck.ExpectKnownValue(exportDataAddr, tfjsonpath.New("scope"), knownvalue.StringExact("user")),
				},
				Check: resource.TestCheckOutput("has_name", "true"),
			},
			{
				Config:            exportDataConfig(testDomain, `scope = "current_host"`),
				ConfigStateChecks: []statecheck.StateCheck{exportPlist(export(defaults.ScopeCurrentHost, testDomain))},
			},
			{
				Config: withProviderScope("current_host", exportDataConfig(testDomain, "")),
				ConfigStateChecks: []statecheck.StateCheck{
					exportPlist(export(defaults.ScopeCurrentHost, testDomain)),
					statecheck.ExpectKnownValue(exportDataAddr, tfjsonpath.New("scope"), knownvalue.StringExact("current_host")),
				},
			},
			{
				// A domain that doesn't exist exports as an empty dict.
				Config:            exportDataConfig("com.example.nothing", ""),
				ConfigStateChecks: []statecheck.StateCheck{exportPlist(export(defaults.ScopeUser, "com.example.nothing"))},
				Check:             resource.TestMatchResourceAttr(exportDataAddr, "plist", regexp.MustCompile(`<dict></dict>|<dict/>`)),
			},
		},
	})
}

func TestExportDataSourceErrors(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(&defaultstest.Fake{}, 501),
		Steps: []resource.TestStep{
			{Config: exportDataConfig("/Library/Preferences/com.apple.dock", ""), ExpectError: regexp.MustCompile(`isn't a supported domain`)},
			{Config: exportDataConfig(testDomain, `scope = "system"`), ExpectError: regexp.MustCompile(`scope requires root`)},
			{Config: exportDataConfig(testDomain, "")},
		},
	})

	failing := &defaultstest.Fake{ExportErr: fmt.Errorf("defaults export failed: Operation not permitted: %w", defaults.ErrPermission)}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(failing, 501),
		Steps: []resource.TestStep{
			{
				Config:      exportDataConfig("com.apple.Safari", ""),
				ExpectError: regexp.MustCompile(`(?s)Exporting preference domain.*com\.apple\.Safari.*Full Disk\s+Access`),
			},
		},
	})

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(rawExport{Fake: &defaultstest.Fake{}, raw: []byte("<plist>\xff</plist>")}, 501),
		Steps: []resource.TestStep{
			{
				Config:      exportDataConfig(testDomain, ""),
				ExpectError: regexp.MustCompile(`isn't\s+valid\s+UTF-8`),
			},
		},
	})
}

func TestExportDataSourceAsRoot(t *testing.T) {
	fake := &defaultstest.Fake{}
	fake.Set(defaults.ScopeSystem, "com.apple.loginwindow", "LoginwindowText", plistvalue.String("hello"))
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 0),
		Steps: []resource.TestStep{
			{Config: exportDataConfig("com.apple.loginwindow", ""), ExpectError: regexp.MustCompile(`scope can't be used as root`)},
			{
				Config: withProviderScope("system", exportDataConfig("com.apple.loginwindow", "")),
				Check:  resource.TestMatchResourceAttr(exportDataAddr, "plist", regexp.MustCompile(`<string>hello</string>`)),
			},
		},
	})
}

func TestAccExportDataSource(t *testing.T) {
	acctest.PreCheckUserScope(t)
	domain := acctest.ScratchDomain(t, defaults.ScopeUser)
	r := defaults.New()
	ctx := context.Background()
	if err := r.Write(ctx, defaults.ScopeUser, domain, "k", plistvalue.Dict(map[string]plistvalue.Value{
		"list": plistvalue.Array(plistvalue.Integer(1), plistvalue.Data([]byte("hi"))),
	})); err != nil {
		t.Fatal(err)
	}
	want, err := r.Export(ctx, defaults.ScopeUser, domain)
	if err != nil {
		t.Fatal(err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:            exportDataConfig(domain, ""),
				ConfigStateChecks: []statecheck.StateCheck{exportPlist(string(want))},
				Check:             resource.TestMatchResourceAttr(exportDataAddr, "plist", regexp.MustCompile(`(?s)^<\?xml .*<key>k</key>.*</plist>\n$`)),
			},
		},
	})
}
