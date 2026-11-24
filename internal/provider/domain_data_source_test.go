package provider

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"testing"
	"time"

	"github.com/demon/terraform-provider-macosdefaults/internal/acctest"
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/defaults/defaultstest"
	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const domainDataAddr = "data.macosdefaults_domain.test"

func domainDataConfig(domain, extra string) string {
	return fmt.Sprintf(`
data "macosdefaults_domain" "test" {
  domain = %q
%s
}
`, domain, extra)
}

func domainValue(key string, v knownvalue.Check) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("values").AtMapKey(key), v)
}

func TestDomainDataSource(t *testing.T) {
	fake := &defaultstest.Fake{}
	u := defaults.ScopeUser
	fake.Set(u, testDomain, "flag", plistvalue.Boolean(true))
	fake.Set(u, testDomain, "count", plistvalue.Integer(3))
	fake.Set(u, testDomain, "size", plistvalue.Float(48))
	fake.Set(u, testDomain, "when", plistvalue.Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)))
	fake.Set(u, testDomain, "blob", plistvalue.Data([]byte("hi")))
	fake.Set(u, testDomain, "NSWindow Frame Main", plistvalue.String("0 0 800 600"))
	fake.Set(u, testDomain, "nested", plistvalue.Dict(map[string]plistvalue.Value{
		"list": plistvalue.Array(plistvalue.String("a"), plistvalue.Integer(1)),
	}))
	fake.Set(defaults.ScopeCurrentHost, testDomain, "flag", plistvalue.Boolean(false))

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config: domainDataConfig(testDomain, "") + `
output "flag" {
  value = data.macosdefaults_domain.test.values.flag
}

output "frame" {
  value = data.macosdefaults_domain.test.values["NSWindow Frame Main"]
}

output "first" {
  value = data.macosdefaults_domain.test.values.nested.list[0]
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("scope"), knownvalue.StringExact("user")),
					domainValue("flag", knownvalue.Bool(true)),
					domainValue("count", knownvalue.NumberExact(big.NewFloat(3))),
					domainValue("size", knownvalue.NumberExact(big.NewFloat(48))),
					domainValue("when", knownvalue.StringExact("2024-01-02T03:04:05Z")),
					domainValue("blob", knownvalue.StringExact("aGk=")),
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("types"), knownvalue.MapExact(map[string]knownvalue.Check{
						"flag":                knownvalue.StringExact("boolean"),
						"count":               knownvalue.StringExact("integer"),
						"size":                knownvalue.StringExact("float"),
						"when":                knownvalue.StringExact("date"),
						"blob":                knownvalue.StringExact("data"),
						"NSWindow Frame Main": knownvalue.StringExact("string"),
						"nested":              knownvalue.StringExact("dict"),
					})),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckOutput("flag", "true"),
					resource.TestCheckOutput("frame", "0 0 800 600"),
					resource.TestCheckOutput("first", "a"),
				),
			},
			{
				Config: domainDataConfig(testDomain, `scope = "current_host"`),
				ConfigStateChecks: []statecheck.StateCheck{
					domainValue("flag", knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("types"), knownvalue.MapSizeExact(1)),
				},
			},
			{
				Config: withProviderScope("current_host", domainDataConfig(testDomain, "")),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("scope"), knownvalue.StringExact("current_host")),
					domainValue("flag", knownvalue.Bool(false)),
				},
			},
			{
				// A domain that doesn't exist reads as empty, not as an error.
				Config: domainDataConfig("com.example.nothing", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("values"), knownvalue.ObjectExact(map[string]knownvalue.Check{})),
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("types"), knownvalue.MapSizeExact(0)),
				},
			},
		},
	})
}

func TestDomainDataSourceErrors(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{Config: domainDataConfig(".GlobalPreferences_m", ""), ExpectError: regexp.MustCompile(`isn't a supported domain`)},
			{Config: domainDataConfig("-g", ""), ExpectError: regexp.MustCompile(`isn't a supported domain`)},
			{Config: domainDataConfig(testDomain, `scope = "system"`), ExpectError: regexp.MustCompile(`scope requires root`)},
			{Config: domainDataConfig(testDomain, "")},
		},
	})

	failing := &defaultstest.Fake{ExportErr: fmt.Errorf("defaults export failed: Operation not permitted: %w", defaults.ErrPermission)}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(failing, 501),
		Steps: []resource.TestStep{
			{
				Config:      domainDataConfig("com.apple.Safari", ""),
				ExpectError: regexp.MustCompile(`(?s)Reading preference domain.*com\.apple\.Safari.*Full Disk\s+Access`),
			},
		},
	})
}

func TestDomainDataSourceAsRoot(t *testing.T) {
	fake := &defaultstest.Fake{}
	fake.Set(defaults.ScopeSystem, "com.apple.loginwindow", "LoginwindowText", plistvalue.String("hello"))
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 0),
		Steps: []resource.TestStep{
			{
				Config:      domainDataConfig("com.apple.loginwindow", ""),
				ExpectError: regexp.MustCompile(`scope can't be used as root`),
			},
			{
				Config:            withProviderScope("system", domainDataConfig("com.apple.loginwindow", "")),
				ConfigStateChecks: []statecheck.StateCheck{domainValue("LoginwindowText", knownvalue.StringExact("hello"))},
			},
		},
	})
}

func TestAccDomainDataSource(t *testing.T) {
	acctest.PreCheckUserScope(t)
	domain := acctest.ScratchDomain(t, defaults.ScopeUser)
	r := defaults.New()
	ctx := context.Background()
	for key, v := range map[string]plistvalue.Value{
		"flag":   plistvalue.Boolean(true),
		"size":   plistvalue.Float(48),
		"nested": plistvalue.Dict(map[string]plistvalue.Value{"list": plistvalue.Array(plistvalue.Integer(1), plistvalue.String("x"))}),
	} {
		if err := r.Write(ctx, defaults.ScopeUser, domain, key, v); err != nil {
			t.Fatal(err)
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: domainDataConfig(domain, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					domainValue("flag", knownvalue.Bool(true)),
					domainValue("size", knownvalue.NumberExact(big.NewFloat(48))),
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("values").AtMapKey("nested").AtMapKey("list").AtSliceIndex(1), knownvalue.StringExact("x")),
					statecheck.ExpectKnownValue(domainDataAddr, tfjsonpath.New("types"), knownvalue.MapExact(map[string]knownvalue.Check{
						"flag":   knownvalue.StringExact("boolean"),
						"size":   knownvalue.StringExact("float"),
						"nested": knownvalue.StringExact("dict"),
					})),
				},
			},
		},
	})
}
