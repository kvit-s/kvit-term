import QtQuick
import QtQuick.Controls
import KvitTerm

ApplicationWindow {
    id: window

    // Set from the command line by main.cpp; empty means the user's shell.
    property string program: ""

    width: 900
    height: 560
    visible: true
    title: terminal.session && terminal.session.title
           ? terminal.session.title
           : qsTr("kvit-term demonstration")
    color: palette.background

    TerminalPalette {
        id: palette
    }

    TerminalSession {
        id: session
        objectName: "session"
        program: window.program
        scrollbackLimit: 10000
        onExited: (code) => {
            status.text = qsTr("the shell exited with status %1 — close the window").arg(code)
            status.visible = true
        }
        onFailed: (message) => {
            status.text = message
            status.visible = true
        }
    }

    TerminalView {
        id: terminal
        objectName: "terminal"
        anchors.fill: parent
        anchors.margins: 6
        anchors.bottomMargin: status.visible ? status.height + 6 : 6

        session: session
        palette: palette
        font.family: "monospace"
        font.pointSize: 11
        focus: true

        // Everything not named here goes to the shell, including the keys an
        // application might otherwise expect for itself.
        reservedShortcuts: ["Ctrl+Shift+N"]
    }

    // Scrolled back through the history, a hint that new output is elsewhere.
    Rectangle {
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 12
        width: label.implicitWidth + 16
        height: label.implicitHeight + 8
        radius: 4
        visible: terminal.scrollOffset > 0
        color: palette.selectionBackground

        Label {
            id: label
            anchors.centerIn: parent
            color: palette.foreground
            text: qsTr("%1 lines back").arg(terminal.scrollOffset)
        }
    }

    Label {
        id: status
        objectName: "status"
        visible: false
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        padding: 6
        color: palette.foreground
        background: Rectangle { color: Qt.darker(palette.background, 1.4) }
    }

    Shortcut {
        sequence: "Ctrl+Shift+N"
        onActivated: {
            status.text = qsTr("Ctrl+Shift+N was reserved by the application, so the shell never saw it")
            status.visible = true
        }
    }
}
