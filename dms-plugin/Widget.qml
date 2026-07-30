import QtQuick
import Quickshell
import qs.Common
import qs.Services
import qs.Widgets
import qs.Modules.Plugins
import "translations.js" as Tr

PluginComponent {
    id: root

    property string lang: Qt.locale().name.split(/[_-]/)[0]
    function tr(key) {
        return Tr.tr(key, lang);
    }

    property int refreshInterval: (pluginData.refreshInterval || 60) * 1000

    property bool hasNext: false
    property string nextName: ""
    property string nextSchedule: ""

    property bool loading: false
    property var pendingJobs: []

    function statusColor(status) {
        if (status === "ativo")
            return Theme.success || "#a6e3a1";
        if (status === "pausado")
            return Theme.warning;
        return Theme.surfaceVariantText;
    }

    function statusLabel(status) {
        return root.tr(status);
    }

    function refresh() {
        root.loading = true;

        Proc.runCommand("djobs.next", ["djobs", "ipc", "jobs.next", "--json"], (stdout, exitCode) => {
            if (exitCode !== 0) {
                root.hasNext = false;
                return;
            }
            try {
                const data = JSON.parse((stdout || "").trim());
                root.hasNext = !!data;
                root.nextName = data ? data.name : "";
                root.nextSchedule = data ? data.schedule_human : "";
            } catch (e) {
                root.hasNext = false;
            }
        }, 3000);

        Proc.runCommand("djobs.list", ["djobs", "ipc", "jobs.list", "pending=true", "--json"], (stdout, exitCode) => {
            root.loading = false;
            if (exitCode !== 0) {
                root.pendingJobs = [];
                return;
            }
            try {
                const data = JSON.parse((stdout || "").trim());
                root.pendingJobs = Array.isArray(data) ? data : [];
            } catch (e) {
                root.pendingJobs = [];
            }
        }, 3000);
    }

    Timer {
        interval: root.refreshInterval
        running: true
        repeat: true
        triggeredOnStart: true
        onTriggered: root.refresh()
    }

    horizontalBarPill: Component {
        Row {
            spacing: Theme.spacingXS

            DankIcon {
                name: "schedule"
                size: iconSize
                color: root.hasNext ? Theme.primary : Theme.surfaceVariantText
                anchors.verticalCenter: parent.verticalCenter
            }

            StyledText {
                text: root.hasNext ? (root.nextName + " · " + root.nextSchedule) : root.tr("sem jobs pendentes")
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceText
                elide: Text.ElideRight
                anchors.verticalCenter: parent.verticalCenter
            }
        }
    }

    verticalBarPill: Component {
        Column {
            spacing: 2

            DankIcon {
                name: "schedule"
                size: iconSize
                color: root.hasNext ? Theme.primary : Theme.surfaceVariantText
                anchors.horizontalCenter: parent.horizontalCenter
            }

            StyledText {
                text: root.pendingJobs.length > 0 ? root.pendingJobs.length.toString() : ""
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceText
                anchors.horizontalCenter: parent.horizontalCenter
            }
        }
    }

    component JobRow: Item {
        property var itemData: null

        width: ListView.view.width
        height: 32

        Row {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            spacing: Theme.spacingS

            Rectangle {
                width: 8
                height: 8
                radius: 4
                color: root.statusColor(itemData ? itemData.status : "")
                anchors.verticalCenter: parent.verticalCenter
            }

            StyledText {
                width: parent.width - 100
                text: itemData ? itemData.name : ""
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceText
                elide: Text.ElideRight
                anchors.verticalCenter: parent.verticalCenter
            }

            StyledText {
                text: itemData ? root.statusLabel(itemData.status) + " · " + itemData.schedule_human : ""
                font.pixelSize: Theme.fontSizeSmall - 2
                color: Theme.surfaceVariantText
                anchors.verticalCenter: parent.verticalCenter
            }
        }
    }

    popoutContent: Component {
        Column {
            width: parent.width
            spacing: Theme.spacingS
            topPadding: Theme.spacingM
            bottomPadding: Theme.spacingM
            leftPadding: Theme.spacingM
            rightPadding: Theme.spacingM

            Item {
                width: parent.width - Theme.spacingM * 2
                height: Math.max(titleText.implicitHeight, refreshRow.implicitHeight)

                StyledText {
                    id: titleText
                    anchors.left: parent.left
                    anchors.verticalCenter: parent.verticalCenter
                    text: root.tr("jobs pendentes") + " (" + root.pendingJobs.length + ")"
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Font.Bold
                    color: Theme.surfaceText
                }

                Row {
                    id: refreshRow
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Theme.spacingXS

                    DankIcon {
                        id: refreshIcon
                        name: "refresh"
                        size: 16
                        color: Theme.primary
                        anchors.verticalCenter: parent.verticalCenter

                        RotationAnimation on rotation {
                            from: 0
                            to: 360
                            duration: 800
                            loops: Animation.Infinite
                            running: root.loading
                        }

                        MouseArea {
                            anchors.fill: parent
                            cursorShape: Qt.PointingHandCursor
                            onClicked: root.refresh()
                        }
                    }

                    StyledText {
                        text: root.tr("atualizar")
                        font.pixelSize: Theme.fontSizeSmall
                        color: Theme.surfaceVariantText
                        anchors.verticalCenter: parent.verticalCenter
                    }
                }
            }

            StyledText {
                visible: !root.loading && root.pendingJobs.length === 0
                text: root.tr("nenhum job pendente")
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceVariantText
            }

            ListView {
                width: parent.width - Theme.spacingM * 2
                height: Math.min(root.pendingJobs.length * 32, 300)
                spacing: 4
                clip: true
                visible: root.pendingJobs.length > 0
                model: root.pendingJobs
                delegate: JobRow {
                    itemData: modelData
                }
            }
        }
    }

    popoutWidth: 300
    popoutHeight: 0
}
