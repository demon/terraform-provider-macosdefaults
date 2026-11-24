# Every domain for the current user.
data "macosdefaults_domains" "user" {}

# Per-machine (ByHost) domains for the current user.
data "macosdefaults_domains" "this_mac" {
  scope = "current_host"
}

output "apple_domains" {
  value = [for d in data.macosdefaults_domains.user.domains : d if startswith(d, "com.apple.")]
}
