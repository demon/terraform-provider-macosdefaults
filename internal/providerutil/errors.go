package providerutil

import (
	"errors"
	"fmt"

	"github.com/demon/terraform-provider-macosdefaults/internal/defaults"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// RunnerError turns a failed defaults operation into a diagnostic that names
// what was being done and where, and explains permission failures.
func RunnerError(summary string, scope defaults.Scope, domain, key string, err error) diag.Diagnostic {
	where := fmt.Sprintf("%s (scope %s)", domain, scope)
	if key != "" {
		where = fmt.Sprintf("key %q in %s", key, where)
	}
	detail := fmt.Sprintf("%s for %s: %s", summary, where, err)

	if errors.Is(err, defaults.ErrPermission) {
		if scope == defaults.ScopeSystem {
			detail += "\n\nSystem-scope preferences need Terraform to run as root."
		} else {
			detail += "\n\nmacOS refused access. Some domains (Safari, Mail and other sandboxed apps) need the app " +
				"running Terraform, such as your terminal, to have Full Disk Access in System Settings → " +
				"Privacy & Security."
		}
	}
	return diag.NewErrorDiagnostic(summary, detail)
}
