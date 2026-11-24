# Booleans, numbers and strings infer their plist type.
resource "macosdefaults_preference" "dock_autohide" {
  domain = "com.apple.dock"
  key    = "autohide"
  value  = true
}

resource "macosdefaults_preference" "key_repeat" {
  domain = "NSGlobalDomain"
  key    = "KeyRepeat"
  value  = 2
}

resource "macosdefaults_preference" "screenshot_location" {
  domain = "com.apple.screencapture"
  key    = "location"
  value  = pathexpand("~/Pictures/Screenshots") # defaults doesn't expand ~
}

# Override inference when an app stores a whole number as a real.
resource "macosdefaults_preference" "dock_tilesize" {
  domain = "com.apple.dock"
  key    = "tilesize"
  value  = 48
  type   = "float"
}

# Lists and maps become arrays and dicts, nested as deeply as needed.
resource "macosdefaults_preference" "finder_view_settings" {
  domain = "com.apple.finder"
  key    = "FXInfoPanesExpanded"
  value = {
    General  = true
    MetaData = false
    Preview  = false
  }
}

# Per-machine (ByHost) preferences.
resource "macosdefaults_preference" "battery_percentage" {
  domain = "com.apple.controlcenter"
  key    = "BatteryShowPercentage"
  value  = true
  scope  = "current_host"
}

# Many apps only read preferences at launch.
resource "terraform_data" "restart_dock" {
  triggers_replace = [
    macosdefaults_preference.dock_autohide.value,
    macosdefaults_preference.dock_tilesize.value,
  ]

  provisioner "local-exec" {
    command = "killall Dock"
  }
}
