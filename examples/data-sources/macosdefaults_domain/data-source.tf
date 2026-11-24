data "macosdefaults_domain" "dock" {
  domain = "com.apple.dock"
}

# Keys that are valid identifiers work with dot syntax; any key works with [].
output "dock_autohide" {
  value = try(data.macosdefaults_domain.dock.values.autohide, null)
}

# `types` tells a float from an integer, or a date from a string.
output "dock_float_keys" {
  value = [for k, t in data.macosdefaults_domain.dock.types : k if t == "float"]
}
