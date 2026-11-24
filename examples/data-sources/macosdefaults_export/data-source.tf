data "macosdefaults_export" "dock" {
  domain = "com.apple.dock"
}

# Keep a copy of the Dock's settings, e.g. before changing them. Restore it
# with `defaults import com.apple.dock backups/com.apple.dock.plist`.
resource "local_file" "dock_backup" {
  filename = "${path.module}/backups/com.apple.dock.plist"
  content  = data.macosdefaults_export.dock.plist
}
