# <scope>/<domain>:<key>, matching the resource's id
terraform import macosdefaults_preference.battery_percentage 'current_host/com.apple.controlcenter:BatteryShowPercentage'

# <domain>:<key> uses the provider's default scope
terraform import macosdefaults_preference.dock_autohide 'com.apple.dock:autohide'
