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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const testDomain = "com.example.tf"

const prefAddr = "macosdefaults_preference.test"

func prefConfig(body string) string {
	return fmt.Sprintf(`
resource "macosdefaults_preference" "test" {
  domain = %q
  key    = "k"
%s
}
`, testDomain, body)
}

// expectStored checks the fake holds want (same kind, equal value) under
// scope/testDomain/key.
func expectStored(fake *defaultstest.Fake, scope defaults.Scope, key string, want plistvalue.Value) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got, ok := fake.Get(scope, testDomain, key)
		if !ok {
			return fmt.Errorf("%s/%s %s not stored", scope, testDomain, key)
		}
		if got.Kind != want.Kind || !plistvalue.Equal(got, want) {
			return fmt.Errorf("stored %+v, want %+v", got, want)
		}
		return nil
	}
}

// expectAbsent checks the fake has no "k" under scope/testDomain.
func expectAbsent(fake *defaultstest.Fake, scope defaults.Scope) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got, ok := fake.Get(scope, testDomain, "k"); ok {
			return fmt.Errorf("%s/%s k still stored as %+v", scope, testDomain, got)
		}
		return nil
	}
}

func stateValue(v knownvalue.Check) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("value"), v)
}

func stateType(kind string) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("type"), knownvalue.StringExact(kind))
}

func TestPreferenceResourceLifecycle(t *testing.T) {
	fake := &defaultstest.Fake{}
	var writesAfterStep2 int

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		CheckDestroy:             expectAbsent(fake, defaults.ScopeUser),
		Steps: []resource.TestStep{
			{
				Config: prefConfig(`value = true`),
				ConfigStateChecks: []statecheck.StateCheck{
					stateValue(knownvalue.Bool(true)),
					stateType("boolean"),
					statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("scope"), knownvalue.StringExact("user")),
					statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("id"), knownvalue.StringExact("user/"+testDomain+":k")),
				},
				Check: expectStored(fake, defaults.ScopeUser, "k", plistvalue.Boolean(true)),
			},
			{
				Config: prefConfig(`value = { a = [1, 2.5, "x"], b = { nested = false } }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(prefAddr, plancheck.ResourceActionUpdate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{stateType("dict")},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectStored(fake, defaults.ScopeUser, "k", plistvalue.Dict(map[string]plistvalue.Value{
						"a": plistvalue.Array(plistvalue.Integer(1), plistvalue.Float(2.5), plistvalue.String("x")),
						"b": plistvalue.Dict(map[string]plistvalue.Value{"nested": plistvalue.Boolean(false)}),
					})),
					func(*terraform.State) error { writesAfterStep2 = fake.Writes(); return nil },
				),
			},
			{
				// Same config again: no diff, and no write.
				Config: prefConfig(`value = { a = [1, 2.5, "x"], b = { nested = false } }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: func(*terraform.State) error {
					if got := fake.Writes(); got != writesAfterStep2 {
						return fmt.Errorf("a no-op apply wrote %d times", got-writesAfterStep2)
					}
					return nil
				},
			},
			{
				Config:            prefConfig(`value = 48` + "\n" + `type = "float"`),
				ConfigStateChecks: []statecheck.StateCheck{stateType("float")},
				Check:             expectStored(fake, defaults.ScopeUser, "k", plistvalue.Float(48)),
			},
			{
				ResourceName:      prefAddr,
				ImportState:       true,
				ImportStateId:     testDomain + ":k",
				ImportStateVerify: true,
			},
			{
				Config:            prefConfig(`value = "2024-01-02T05:04:05+02:00"` + "\n" + `type = "date"`),
				ConfigStateChecks: []statecheck.StateCheck{stateType("date")},
				Check:             expectStored(fake, defaults.ScopeUser, "k", plistvalue.Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))),
			},
			{
				Config:            prefConfig(`value = base64encode("hi")` + "\n" + `type = "data"`),
				ConfigStateChecks: []statecheck.StateCheck{stateType("data")},
				Check:             expectStored(fake, defaults.ScopeUser, "k", plistvalue.Data([]byte("hi"))),
			},
			{
				Config: prefConfig(`value = ["only", "strings"]`),
				ConfigStateChecks: []statecheck.StateCheck{
					stateType("array"),
					stateValue(knownvalue.TupleExact([]knownvalue.Check{knownvalue.StringExact("only"), knownvalue.StringExact("strings")})),
				},
			},
		},
	})
}

func TestPreferenceResourceDrift(t *testing.T) {
	fake := &defaultstest.Fake{}
	set := func(v plistvalue.Value) func() {
		return func() { fake.Set(defaults.ScopeUser, testDomain, "k", v) }
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{Config: prefConfig(`value = true`)},
			{
				// The app rewrote the boolean as an integer: not drift.
				PreConfig: set(plistvalue.Integer(1)),
				Config:    prefConfig(`value = true`),
				PlanOnly:  true,
			},
			{
				// Changed outside Terraform: drift, planned as an update.
				PreConfig:          set(plistvalue.Boolean(false)),
				Config:             prefConfig(`value = true`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// Applying fixes it.
				Config: prefConfig(`value = true`),
				Check:  expectStored(fake, defaults.ScopeUser, "k", plistvalue.Boolean(true)),
			},
			{
				// Same text, but now a string on disk: drift.
				PreConfig:          set(plistvalue.String("true")),
				Config:             prefConfig(`value = true`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// Deleted outside Terraform: planned as a create.
				PreConfig: func() { fake.Remove(defaults.ScopeUser, testDomain, "k") },
				Config:    prefConfig(`value = true`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(prefAddr, plancheck.ResourceActionCreate)},
				},
				Check: expectStored(fake, defaults.ScopeUser, "k", plistvalue.Boolean(true)),
			},
		},
	})
}

func TestPreferenceResourceIntegerFloatDrift(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{Config: prefConfig(`value = 48`), ConfigStateChecks: []statecheck.StateCheck{stateType("integer")}},
			{
				PreConfig:          func() { fake.Set(defaults.ScopeUser, testDomain, "k", plistvalue.Float(48)) },
				Config:             prefConfig(`value = 48`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// Declaring the type the app uses settles it.
				Config: prefConfig(`value = 48` + "\n" + `type = "float"`),
				Check:  expectStored(fake, defaults.ScopeUser, "k", plistvalue.Float(48)),
			},
		},
	})
}

func TestPreferenceResourceUnknownValue(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config: `
resource "terraform_data" "src" {
  input = 7
}

resource "macosdefaults_preference" "test" {
  domain = "` + testDomain + `"
  key    = "k"
  value  = terraform_data.src.output
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectUnknownValue(prefAddr, tfjsonpath.New("type"))},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					stateType("integer"),
					stateValue(knownvalue.NumberExact(big.NewFloat(7))),
				},
				Check: expectStored(fake, defaults.ScopeUser, "k", plistvalue.Integer(7)),
			},
		},
	})
}

func TestPreferenceResourceScopeChangeReplaces(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{Config: prefConfig(`value = "a"`)},
			{
				Config: prefConfig(`value = "a"` + "\n" + `scope = "current_host"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(prefAddr, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("id"), knownvalue.StringExact("current_host/"+testDomain+":k")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectAbsent(fake, defaults.ScopeUser),
					expectStored(fake, defaults.ScopeCurrentHost, "k", plistvalue.String("a")),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "macosdefaults_preference" "test" {
  domain = %q
  key    = "k2"
  scope  = "current_host"
  value  = "a"
}
`, testDomain),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(prefAddr, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectAbsent(fake, defaults.ScopeCurrentHost),
					expectStored(fake, defaults.ScopeCurrentHost, "k2", plistvalue.String("a")),
				),
			},
		},
	})
}

func TestPreferenceResourceValidation(t *testing.T) {
	fake := &defaultstest.Fake{}
	bad := []struct {
		config string
		err    string
	}{
		{prefConfig(`value = 2.5` + "\n" + `type = "integer"`), `whole number`},
		{prefConfig(`value = "x"` + "\n" + `type = "boolean"`), `doesn't match a string`},
		{prefConfig(`value = "tomorrow"` + "\n" + `type = "date"`), `RFC 3339`},
		{prefConfig(`value = "2024-01-02T03:04:05.25Z"` + "\n" + `type = "date"`), `one-second`},
		{prefConfig(`value = "not base64!"` + "\n" + `type = "data"`), `base64`},
		{prefConfig(`value = ["a", null]`), `null values`},
		{prefConfig(`value = "x"` + "\n" + `type = "blob"`), `value must be one of`},
		{prefConfig(`value = "x"` + "\n" + `scope = "everywhere"`), `value must be one of`},
		{`
resource "macosdefaults_preference" "test" {
  domain = "-g"
  key    = "k"
  value  = true
}`, `isn't a supported domain`},
		{`
resource "macosdefaults_preference" "test" {
  domain = ".GlobalPreferences_m"
  key    = "AppleLocale"
  value  = "en_US"
}`, `isn't a supported domain`},
		{`
resource "macosdefaults_preference" "test" {
  domain = "` + testDomain + `"
  key    = "-string"
  value  = true
}`, `isn't a supported key`},
	}

	var steps []resource.TestStep
	for _, b := range bad {
		steps = append(steps, resource.TestStep{Config: b.config, ExpectError: regexp.MustCompile(b.err)})
	}
	// End on a valid config: the post-test destroy reuses the last step's.
	steps = append(steps, resource.TestStep{Config: prefConfig(`value = "ok"`)})

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps:                    steps,
	})
	if got := fake.Writes(); got != 1 {
		t.Errorf("invalid configs caused writes: %d writes, want only the final valid one", got)
	}
}

func TestPreferenceResourceAsRoot(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 0),
		CheckDestroy:             expectAbsent(fake, defaults.ScopeSystem),
		Steps: []resource.TestStep{
			{
				Config:      prefConfig(`value = true`),
				ExpectError: regexp.MustCompile(`scope can't be used as root`),
			},
			{
				Config:      prefConfig(`value = true` + "\n" + `scope = "current_host"`),
				ExpectError: regexp.MustCompile(`scope can't be used as root`),
			},
			{
				Config: prefConfig(`value = true` + "\n" + `scope = "system"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("id"), knownvalue.StringExact("system/"+testDomain+":k")),
				},
				Check: expectStored(fake, defaults.ScopeSystem, "k", plistvalue.Boolean(true)),
			},
			{
				ResourceName:      prefAddr,
				ImportState:       true,
				ImportStateId:     "system/" + testDomain + ":k",
				ImportStateVerify: true,
			},
		},
	})
	if fake.Writes() != 1 {
		t.Errorf("got %d writes, want 1 (the system-scope apply only)", fake.Writes())
	}
}

func TestPreferenceResourceSystemAsNonRoot(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config:      prefConfig(`value = true` + "\n" + `scope = "system"`),
				ExpectError: regexp.MustCompile(`scope requires root`),
			},
			{Config: prefConfig(`value = true`)},
		},
	})
}

func TestPreferenceResourceRunnerErrors(t *testing.T) {
	fake := &defaultstest.Fake{WriteErr: fmt.Errorf("defaults write failed: Operation not permitted: %w", defaults.ErrPermission)}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config:      prefConfig(`value = true`),
				ExpectError: regexp.MustCompile(`(?s)Writing preference.*Full Disk\s+Access`),
			},
		},
	})
}

func TestParsePreferenceID(t *testing.T) {
	good := []struct {
		id     string
		scope  defaults.Scope
		domain string
		key    string
	}{
		{"com.apple.dock:autohide", "", "com.apple.dock", "autohide"},
		{"user/com.apple.dock:autohide", defaults.ScopeUser, "com.apple.dock", "autohide"},
		{"current_host/com.apple.controlcenter:Battery Show", defaults.ScopeCurrentHost, "com.apple.controlcenter", "Battery Show"},
		{"system/NSGlobalDomain:AppleLocale", defaults.ScopeSystem, "NSGlobalDomain", "AppleLocale"},
		{"user/com.example:key:with:colons", defaults.ScopeUser, "com.example", "key:with:colons"},
		{"com.example:key/with:slash", "", "com.example", "key/with:slash"},
	}
	for _, tt := range good {
		scope, domain, key, err := parsePreferenceID(tt.id)
		if err != nil {
			t.Errorf("parsePreferenceID(%q): %v", tt.id, err)
			continue
		}
		if scope != tt.scope || domain != tt.domain || key != tt.key {
			t.Errorf("parsePreferenceID(%q) = %s, %q, %q; want %s, %q, %q", tt.id, scope, domain, key, tt.scope, tt.domain, tt.key)
		}
		if scope != "" {
			if round := preferenceID(scope, domain, key); round != tt.id {
				t.Errorf("preferenceID round trip of %q = %q", tt.id, round)
			}
		}
	}
	for _, id := range []string{"", "com.apple.dock", ":k", "com.apple.dock:", "-g:k", "bogus/com.apple.dock:k", "/etc/x:k", "com.apple.dock:-k"} {
		if _, _, _, err := parsePreferenceID(id); err == nil {
			t.Errorf("parsePreferenceID(%q) succeeded, want error", id)
		}
	}
}

// Acceptance tests: real defaults, scratch domains only.

func acceptanceConfig(domain, scope, body string) string {
	return fmt.Sprintf(`
resource "macosdefaults_preference" "test" {
  domain = %q
  key    = "k"
  scope  = %q
%s
}
`, domain, scope, body)
}

func expectOnDisk(scope defaults.Scope, domain string, want *plistvalue.Value) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got, ok, err := readKey(context.Background(), defaults.New(), scope, domain, "k")
		if err != nil {
			return err
		}
		switch {
		case want == nil && ok:
			return fmt.Errorf("%s k is still set to %+v", domain, got)
		case want != nil && !ok:
			return fmt.Errorf("%s k isn't set", domain)
		case want != nil && (got.Kind != want.Kind || !plistvalue.Equal(got, *want)):
			return fmt.Errorf("%s k = %+v, want %+v", domain, got, *want)
		}
		return nil
	}
}

func ptrValue(v plistvalue.Value) *plistvalue.Value { return &v }

func testAccPreferenceLifecycle(t *testing.T, scope defaults.Scope) {
	domain := acctest.ScratchDomain(t, scope)
	s := string(scope)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             expectOnDisk(scope, domain, nil),
		Steps: []resource.TestStep{
			{
				Config: acceptanceConfig(domain, s, `value = true`),
				Check:  expectOnDisk(scope, domain, ptrValue(plistvalue.Boolean(true))),
			},
			{
				Config: acceptanceConfig(domain, s, `value = { list = [1, 2.5, "x & <y>"], flag = false }`),
				Check: expectOnDisk(scope, domain, ptrValue(plistvalue.Dict(map[string]plistvalue.Value{
					"list": plistvalue.Array(plistvalue.Integer(1), plistvalue.Float(2.5), plistvalue.String("x & <y>")),
					"flag": plistvalue.Boolean(false),
				}))),
			},
			{
				Config: acceptanceConfig(domain, s, "value = 48\ntype = \"float\""),
				Check:  expectOnDisk(scope, domain, ptrValue(plistvalue.Float(48))),
			},
			{
				ResourceName:      prefAddr,
				ImportState:       true,
				ImportStateId:     preferenceID(scope, domain, "k"),
				ImportStateVerify: true,
			},
			{
				Config: acceptanceConfig(domain, s, "value = \"2024-01-02T03:04:05Z\"\ntype = \"date\""),
				Check:  expectOnDisk(scope, domain, ptrValue(plistvalue.Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)))),
			},
		},
	})
}

func TestAccPreferenceResource(t *testing.T) {
	acctest.PreCheckUserScope(t)
	testAccPreferenceLifecycle(t, defaults.ScopeUser)
}

func TestAccPreferenceResourceDrift(t *testing.T) {
	acctest.PreCheckUserScope(t)
	domain := acctest.ScratchDomain(t, defaults.ScopeUser)
	runner := defaults.New()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: acceptanceConfig(domain, "user", `value = true`)},
			{
				PreConfig: func() {
					if err := runner.Write(context.Background(), defaults.ScopeUser, domain, "k", plistvalue.Integer(1)); err != nil {
						t.Fatal(err)
					}
				},
				Config:   acceptanceConfig(domain, "user", `value = true`),
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					if err := runner.Write(context.Background(), defaults.ScopeUser, domain, "k", plistvalue.String("changed")); err != nil {
						t.Fatal(err)
					}
				},
				Config:             acceptanceConfig(domain, "user", `value = true`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: acceptanceConfig(domain, "user", `value = true`),
				Check:  expectOnDisk(defaults.ScopeUser, domain, ptrValue(plistvalue.Boolean(true))),
			},
		},
	})
}

func TestAccPreferenceResourceCurrentHost(t *testing.T) {
	acctest.PreCheckCurrentHost(t)
	testAccPreferenceLifecycle(t, defaults.ScopeCurrentHost)
}

func TestAccSystemPreferenceResource(t *testing.T) {
	acctest.PreCheckSystem(t)
	testAccPreferenceLifecycle(t, defaults.ScopeSystem)
}

func withProviderScope(scope, config string) string {
	return fmt.Sprintf("provider \"macosdefaults\" {\n  scope = %q\n}\n", scope) + config
}

func TestPreferenceResourceProviderDefaultScope(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 0),
		CheckDestroy:             expectAbsent(fake, defaults.ScopeSystem),
		Steps: []resource.TestStep{
			{
				// An override is still checked against the root rule.
				Config:      withProviderScope("system", prefConfig(`value = true`+"\n"+`scope = "current_host"`)),
				ExpectError: regexp.MustCompile(`scope can't be used as root`),
			},
			{
				Config: withProviderScope("system", prefConfig(`value = true`)),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("scope"), knownvalue.StringExact("system")),
					statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("id"), knownvalue.StringExact("system/"+testDomain+":k")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectStored(fake, defaults.ScopeSystem, "k", plistvalue.Boolean(true)),
					expectAbsent(fake, defaults.ScopeUser),
				),
			},
			{
				// An unprefixed import ID means the provider's default scope.
				Config:            withProviderScope("system", prefConfig(`value = true`)),
				ResourceName:      prefAddr,
				ImportState:       true,
				ImportStateId:     testDomain + ":k",
				ImportStateVerify: true,
			},
		},
	})
}

func TestPreferenceResourceProviderDefaultChange(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config: prefConfig(`value = "a"`),
				Check:  expectStored(fake, defaults.ScopeUser, "k", plistvalue.String("a")),
			},
			{
				// Changing the provider default moves every resource that
				// doesn't set its own scope.
				Config: withProviderScope("current_host", prefConfig(`value = "a"`)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(prefAddr, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(prefAddr, tfjsonpath.New("id"), knownvalue.StringExact("current_host/"+testDomain+":k")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectAbsent(fake, defaults.ScopeUser),
					expectStored(fake, defaults.ScopeCurrentHost, "k", plistvalue.String("a")),
				),
			},
			{
				// Same effective scope spelled differently: an explicit scope
				// with the provider default back to user. No change.
				Config: prefConfig(`value = "a"` + "\n" + `scope = "current_host"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// The resource's own scope overrides the provider's.
				Config: withProviderScope("current_host", prefConfig(`value = "a"`+"\n"+`scope = "user"`)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(prefAddr, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectStored(fake, defaults.ScopeUser, "k", plistvalue.String("a")),
					expectAbsent(fake, defaults.ScopeCurrentHost),
				),
			},
		},
	})
}

func TestPreferenceResourceProviderSystemAsNonRoot(t *testing.T) {
	fake := &defaultstest.Fake{}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(fake, 501),
		Steps: []resource.TestStep{
			{
				Config:      withProviderScope("system", prefConfig(`value = true`)),
				ExpectError: regexp.MustCompile(`scope requires root`),
			},
			{
				// Overriding back to a user scope is fine.
				Config: withProviderScope("system", prefConfig(`value = true`+"\n"+`scope = "user"`)),
				Check:  expectStored(fake, defaults.ScopeUser, "k", plistvalue.Boolean(true)),
			},
		},
	})
}

func TestPreferenceResourceProviderScopeValidation(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: fakeFactories(&defaultstest.Fake{}, 501),
		Steps: []resource.TestStep{
			{
				Config:      withProviderScope("everywhere", prefConfig(`value = true`)),
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
			{Config: prefConfig(`value = true`)},
		},
	})
}
