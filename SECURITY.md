# Security Policy

## Reporting a vulnerability

Please don't open a public issue. Report it privately through
[GitHub's vulnerability reporting](https://github.com/demon/terraform-provider-macosdefaults/security/advisories/new)
and include a minimal reproduction plus the provider and macOS versions.

Examples of what counts:

- Getting a domain, key, or value interpreted as a shell command or a `defaults` flag.
- Making the provider write or delete something the config didn't declare, or write outside
  `/Library/Preferences` in system scope.
- Preference values leaking into logs.

Bugs in macOS, `defaults`, or `cfprefsd` should go to Apple.

## Things to keep in mind as a user

- **Review modules before applying them.** A module using this provider can hange any preference
  you can, including security-related ones like screen lock and Gatekeeper settings. Under `sudo`,
  it can change them for every user, and every other provider in that config runs as root too.
- **State can contain private data.** The `macosdefaults_domain` and `macosdefaults_export` data
  sources copy whole domains into state, which may include file paths, account names, or the odd
  token. Treat that state as sensitive.
- **The provider does what it's told.** If your config turns off a security feature, that's not a
  vulnerability.
