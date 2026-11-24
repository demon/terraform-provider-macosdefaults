# terraform-provider-macosdefaults

[![Tests](https://github.com/demon/terraform-provider-macosdefaults/actions/workflows/test.yml/badge.svg)](https://github.com/demon/terraform-provider-macosdefaults/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/demon/terraform-provider-macosdefaults.svg)](https://pkg.go.dev/github.com/demon/terraform-provider-macosdefaults)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Terraform provider for managing macOS user preferences. It wraps the built-in
[`defaults(1)`](https://ss64.com/mac/defaults.html) tool, so things like Dock behaviour, Finder
options, key repeat rates, and screenshot locations can be declared in Terraform, reviewed in a
plan, and checked for drift, instead of living in a pile of `defaults write` lines in a dotfiles
script.

Published on the [Terraform Registry](https://registry.terraform.io/providers/demon/macosdefaults/latest) and the [OpenTofu Registry](https://search.opentofu.org/provider/demon/macosdefaults/latest) as `demon/macosdefaults`.

## Requirements

- macOS. The provider shells out to `/usr/bin/defaults` on the machine running Terraform and
  refuses to configure on any other OS.
- Terraform 1.x or OpenTofu.

## Install

```terraform
terraform {
  required_providers {
    macosdefaults = {
      source  = "demon/macosdefaults"
      version = "~> 0.1"
    }
  }
}

provider "macosdefaults" {}
```

No credentials needed. The only provider setting is an optional default [`scope`](#scopes).

## Usage

Each `macosdefaults_preference` manages one key in one preference domain. The plist type is
inferred from the HCL value:

```terraform
resource "macosdefaults_preference" "dock_autohide" {
  domain = "com.apple.dock"
  key    = "autohide"
  value  = true # boolean
}

resource "macosdefaults_preference" "key_repeat" {
  domain = "NSGlobalDomain"
  key    = "KeyRepeat"
  value  = 2 # integer
}

resource "macosdefaults_preference" "screenshot_location" {
  domain = "com.apple.screencapture"
  key    = "location"
  value  = pathexpand("~/Pictures/Screenshots") # string; defaults does not expand ~
}

resource "macosdefaults_preference" "dock_tilesize" {
  domain = "com.apple.dock"
  key    = "tilesize"
  value  = 48
  type   = "float" # override inference when an app expects a <real>
}
```

Lists become arrays and maps become dicts. Dates and binary data need an explicit `type` (`"date"`
with an [RFC 3339](https://www.rfc-editor.org/info/rfc3339/) string, `"data"` with base64).

Destroying a resource deletes just that key, so the app falls back to its built-in default.

### Scopes

`scope` picks where a key lives:

| `scope`        | Writes to                                                     | Run Terraform as        |
| -------------- | ------------------------------------------------------------- | ----------------------- |
| `user`         | your preferences (`defaults write`)                           | yourself                |
| `current_host` | your preferences for this Mac only (`defaults -currentHost write`) | yourself           |
| `system`       | `/Library/Preferences/<domain>`, for all users                | root (`sudo terraform`) |

The default is `user`; set `scope` on the provider or on individual resources to change it.
Per-machine keys are the usual override:

```terraform
resource "macosdefaults_preference" "battery_percentage" {
  domain = "com.apple.controlcenter"
  key    = "BatteryShowPercentage"
  value  = true
  scope  = "current_host"
}
```

The provider *never* elevates itself, and one configuration can't mix `system` with the other two.
Under `sudo`, a `user` key would quietly land in root's preferences instead of yours, so the
provider refuses at plan time. Keep system-wide settings in their own root module and apply it
with `sudo terraform apply`:

```terraform
provider "macosdefaults" {
  scope = "system"
}

resource "macosdefaults_preference" "login_window_text" {
  domain = "com.apple.loginwindow"
  key    = "LoginwindowText"
  value  = "Property of Example Co."
}
```

Changing a resource's scope (including via the provider default) replaces it: the key is deleted
from the old scope and written to the new one.

### Making changes take effect

Many apps only read preferences at launch, and the provider doesn't restart anything. To restart
an app when a value changes, use `terraform_data`:

```terraform
resource "terraform_data" "restart_dock" {
  triggers_replace = [macosdefaults_preference.dock_autohide.value]

  provisioner "local-exec" {
    command = "killall Dock"
  }
}
```

### Looking things up

Data sources read preferences without managing them. Handy for finding the key behind a System
Settings toggle:

```terraform
# Every domain for the current user
data "macosdefaults_domains" "all" {}

# A whole domain as native Terraform values
data "macosdefaults_domain" "dock" {
  domain = "com.apple.dock"
}
# data.macosdefaults_domain.dock.values.autohide is a bool, and
# data.macosdefaults_domain.dock.types.autohide is "boolean"

# The same domain as the raw XML that `defaults export` prints
data "macosdefaults_export" "dock" {
  domain = "com.apple.dock"
}

resource "local_file" "dock_backup" {
  filename = "${path.module}/backups/com.apple.dock.plist"
  content  = data.macosdefaults_export.dock.plist
}

# A single key
data "macosdefaults_preference" "key_repeat" {
  domain = "NSGlobalDomain"
  key    = "KeyRepeat"
}
```

Full reference docs are in [`docs/`](docs/) and on the Registry.

## Troubleshooting

- **"Operation not permitted" on some Apple domains** (Safari, Mail, and other sandboxed apps): the
  terminal running Terraform needs Full Disk Access in System Settings → Privacy & Security.
- **The value changed but the app ignores it:** relaunch the app. Some settings need a logout.
- **Perpetual diff on a key:** the app is probably rewriting the value with a different type. Set
  `type` explicitly to match what `defaults read-type <domain> <key>` reports.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues go through [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
