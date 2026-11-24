data "macosdefaults_preference" "key_repeat" {
  domain = "NSGlobalDomain"
  key    = "KeyRepeat"
}

output "key_repeat" {
  value = "${data.macosdefaults_preference.key_repeat.value} (${data.macosdefaults_preference.key_repeat.type})"
}
