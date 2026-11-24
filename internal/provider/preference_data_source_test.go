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

const prefDataAddr = "data.macosdefaults_preference.test"

func prefDataConfig(key, extra string) string {
	return fmt.Sprintf(`
data "macosdefaults_preference" "test" {
  domain = %q
  key    = %q
%s
}
`, testDomain, key, extra)
}

func dataValue(v knownvalue.Check) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(prefDataAddr, tfjsonpath.New("value"), v)
}

func dataType(kind string) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(prefDataAddr, tfjsonpath.New("type"), knownvalue.StringExact(kind))
}

func TestPreferenceDataSource(t *testing.T) {
	fake := &defaultstest.Fake{}
	u := defaults.ScopeUser
	fake.Set(u, testDomain, "flag", plistvalue.Boolean(true))
	fake.Set(u, testDomain, "count", plistvalue.Integer(3))
	fake.Set(u, testDomain, "size", plistvalue.Float(48))
	fake.Set(u, testDomain, "when", plistvalue.Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)))
	fake.Set(u, testDomain, "blob", plistvalue.Data([]byte("hi")))
	fake.Set(u, testDomain, "nested", plistvalue.Dict(map[string]plistvalue.Value{
		"list": plistvalue.Array(plistvalue.String("a"), plistvalue.Integer(1)),
	}))
	fake.Set(defaults.ScopeCurrentHost, testDomain, "flag", plistvalue.Boolean(false))

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config: prefDataConfig("flag", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					dataValue(knownvalue.Bool(true)), dataType("boolean"),
					statecheck.ExpectKnownValue(prefDataAddr, tfjsonpath.New("scope"), knownvalue.StringExact("user")),
				},
			},
			{
				Config:            prefDataConfig("flag", `scope = "current_host"`),
				ConfigStateChecks: []statecheck.StateCheck{dataValue(knownvalue.Bool(false))},
			},
			{
				Config:            prefDataConfig("count", ""),
				ConfigStateChecks: []statecheck.StateCheck{dataValue(knownvalue.NumberExact(big.NewFloat(3))), dataType("integer")},
			},
			{
				Config:            prefDataConfig("size", ""),
				ConfigStateChecks: []statecheck.StateCheck{dataValue(knownvalue.NumberExact(big.NewFloat(48))), dataType("float")},
			},
			{
				Config:            prefDataConfig("when", ""),
				ConfigStateChecks: []statecheck.StateCheck{dataValue(knownvalue.StringExact("2024-01-02T03:04:05Z")), dataType("date")},
			},
			{
				Config:            prefDataConfig("blob", ""),
				ConfigStateChecks: []statecheck.StateCheck{dataValue(knownvalue.StringExact("aGk=")), dataType("data")},
			},
			{
				Config: prefDataConfig("nested", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					dataType("dict"),
					statecheck.ExpectKnownValue(prefDataAddr, tfjsonpath.New("value").AtMapKey("list").AtSliceIndex(0), knownvalue.StringExact("a")),
				},
			},
		},
	})
}

func TestPreferenceDataSourceErrors(t *testing.T) {
	fake := &defaultstest.Fake{}
	fake.Set(defaults.ScopeUser, testDomain, "flag", plistvalue.Boolean(true))

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{Config: prefDataConfig("missing", ""), ExpectError: regexp.MustCompile(`Preference not found`)},
			{Config: prefDataConfig("flag", `scope = "system"`), ExpectError: regexp.MustCompile(`scope requires root`)},
			{Config: prefDataConfig("-flag", ""), ExpectError: regexp.MustCompile(`isn't a supported key`)},
			{Config: prefDataConfig("flag", "")},
		},
	})
}

func TestAccPreferenceDataSource(t *testing.T) {
	acctest.PreCheckUserScope(t)
	domain := acctest.ScratchDomain(t, defaults.ScopeUser)
	want := plistvalue.Dict(map[string]plistvalue.Value{"a": plistvalue.Array(plistvalue.Integer(1), plistvalue.Boolean(true))})
	if err := defaults.New().Write(context.Background(), defaults.ScopeUser, domain, "k", want); err != nil {
		t.Fatal(err)
	}

	config := func(key string) string {
		return fmt.Sprintf(`
data "macosdefaults_preference" "test" {
  domain = %q
  key    = %q
}
`, domain, key)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("missing"), ExpectError: regexp.MustCompile(`Preference not found`)},
			{
				Config: config("k"),
				ConfigStateChecks: []statecheck.StateCheck{
					dataType("dict"),
					statecheck.ExpectKnownValue(prefDataAddr, tfjsonpath.New("value").AtMapKey("a").AtSliceIndex(1), knownvalue.Bool(true)),
				},
			},
		},
	})
}

func TestPreferenceDataSourceProviderDefaultScope(t *testing.T) {
	fake := &defaultstest.Fake{}
	fake.Set(defaults.ScopeUser, testDomain, "flag", plistvalue.Boolean(true))
	fake.Set(defaults.ScopeCurrentHost, testDomain, "flag", plistvalue.Boolean(false))

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config: withProviderScope("current_host", prefDataConfig("flag", "")),
				ConfigStateChecks: []statecheck.StateCheck{
					dataValue(knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(prefDataAddr, tfjsonpath.New("scope"), knownvalue.StringExact("current_host")),
				},
			},
			{
				Config:            withProviderScope("current_host", prefDataConfig("flag", `scope = "user"`)),
				ConfigStateChecks: []statecheck.StateCheck{dataValue(knownvalue.Bool(true))},
			},
		},
	})
}
