## 0.1.0

Initial release.

- Resource `macosdefaults_preference` manages a single key in a preference domain, with type inference from HCL values and an optional `type` override.
- `scope` selects `user`, `current_host` (ByHost), or `system` (`/Library/Preferences`, requires root) preferences. Set a default on the provider and override it per resource or data source; the provider's default is `user`.
- Data sources:
  - `macosdefaults_preference` reads one key.
  - `macosdefaults_domains` lists domains.
  - `macosdefaults_domain` reads a whole domain as native Terraform values, plus each key's plist type.
  - `macosdefaults_export` returns a domain's raw `defaults export` XML.
- Contributor tooling: `prek` Git hooks (`prek.toml`) run formatting, lint and docs checks before commits and pushes.
