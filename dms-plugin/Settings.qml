import QtQuick
import qs.Common
import qs.Modules.Plugins
import qs.Widgets

PluginSettings {
    id: root
    pluginId: "djobs"

    StyledText {
        width: parent.width
        text: "Dank Jobs"
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Font.Bold
        color: Theme.surfaceText
    }

    SliderSetting {
        settingKey: "maxWidth"
        label: "Max name width"
        description: "Maximum width in pixels for the job name in the bar pill. 0 = no limit (names can make the pill as wide as they need)."
        defaultValue: 0
        minimum: 0
        maximum: 400
        stepSize: 10
        unit: "px"
    }

    SliderSetting {
        settingKey: "refreshInterval"
        label: "Refresh interval"
        description: "How often to re-query djobs."
        defaultValue: 60
        minimum: 10
        maximum: 3600
        stepSize: 10
        unit: "s"
    }
}
