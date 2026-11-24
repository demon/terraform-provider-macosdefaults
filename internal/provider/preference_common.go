package provider

import (
	"context"
	"fmt"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/demon/terraform-provider-macosdefaults/internal/plistvalue"
)

// readKey exports domain and returns key's value. ok is false when the key
// (or the whole domain, which exports as an empty dict) doesn't exist.
func readKey(ctx context.Context, r defaults.Runner, scope defaults.Scope, domain, key string) (v plistvalue.Value, ok bool, err error) {
	doc, err := r.Export(ctx, scope, domain)
	if err != nil {
		return plistvalue.Value{}, false, err
	}
	keys, err := plistvalue.DecodeDomain(doc)
	if err != nil {
		return plistvalue.Value{}, false, fmt.Errorf("parsing `defaults export %s` output: %w", domain, err)
	}
	v, ok = keys[key]
	return v, ok, nil
}

// preferenceID is the resource ID: <scope>/<domain>:<key>. It always names
// the scope, because an import ID without one means the provider's default,
// which can differ between configurations.
func preferenceID(scope defaults.Scope, domain, key string) string {
	return string(scope) + "/" + domain + ":" + key
}
