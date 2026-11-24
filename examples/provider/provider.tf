terraform {
  required_providers {
    macosdefaults = {
      source  = "registry.terraform.io/demon/macosdefaults"
      version = "~> 0.1"
    }
  }
}

# The provider runs /usr/bin/defaults as the user running Terraform, so it
# only works on macOS. It takes no credentials.
provider "macosdefaults" {
  # Default scope for resources and data sources that don't set their own:
  # "user" (the default), "current_host", or "system" (needs `sudo terraform`).
  scope = "user"
}
