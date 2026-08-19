import QtQuick
import qs.Common
import qs.Widgets
import qs.Modules.Plugins

PluginSettings {
    id: root
    pluginId: "tjobs"

    StyledText {
        width: parent.width
        text: "Tabela Jobs"
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
        unit: "px"
    }

    SliderSetting {
        settingKey: "refreshInterval"
        label: "Refresh interval"
        description: "How often to re-query tjobs."
        defaultValue: 60
        minimum: 10
        maximum: 3600
        unit: "s"
    }
}
