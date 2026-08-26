import QtQuick
import QtQuick.Controls
import KvitTerm

ApplicationWindow {
    id: window

    // Both set from main.cpp.
    property string program: ""
    property var shellArguments: []

    width: 900
    height: 560
    visible: true
    title: session.title !== "" ? session.title : qsTr("kvit-term demonstration")
    color: colours.background

    TerminalPalette {
        id: colours
    }

    TerminalSession {
        id: session
        objectName: "session"
        program: window.program
        arguments: window.shellArguments
        scrollbackLimit: 10000
        onExited: (code) => note(qsTr("the shell exited with status %1").arg(code))
        onFailed: (message) => note(message)
    }

    // What the shell says about itself: where each command began and ended,
    // what it exited with, and which directory it is in. Empty unless the
    // shell has the integration snippet, which main.cpp installs for bash.
    ShellIntegration {
        id: shell
        session: session
        onCommandFinished: (command, exitCode) => {
            if (exitCode > 0)
                note(qsTr("\"%1\" exited with status %2").arg(command).arg(exitCode))
        }
    }

    TerminalSearch {
        id: search
        session: session
    }

    function note(text) {
        status.text = text
        status.visible = true
    }

    header: ToolBar {
        visible: shell.active || search.query !== ""
        height: visible ? implicitHeight : 0

        Row {
            anchors.fill: parent
            anchors.margins: 6
            spacing: 12

            // Sticky heading: which command produced the output at the top of
            // the window. It needs the shell integration above; without it
            // there is no way to know where one command's output ends.
            Label {
                anchors.verticalCenter: parent.verticalCenter
                visible: terminal.stickyCommand !== ""
                color: colours.foreground
                elide: Text.ElideRight
                width: Math.min(implicitWidth, window.width / 2)
                text: "$ " + terminal.stickyCommand
            }

            Label {
                anchors.verticalCenter: parent.verticalCenter
                visible: shell.currentDirectory !== ""
                opacity: 0.7
                color: colours.foreground
                text: shell.currentDirectory
            }
        }
    }

    TerminalView {
        id: terminal
        objectName: "terminal"
        anchors.fill: parent
        anchors.margins: 6
        anchors.bottomMargin: (status.visible ? status.height : 0)
                              + (findBar.visible ? findBar.height : 0) + 6

        session: session
        palette: colours
        search: search
        shellIntegration: shell
        font.family: "monospace"
        font.pointSize: 11
        focus: true

        // The application keeps these; everything else goes to the shell.
        reservedShortcuts: ["Ctrl+Shift+F", "Ctrl+Shift+N"]

        // Ctrl+click on a path or an address. What to do with one is the
        // application's decision, which is why the library only reports it.
        onLinkActivated: (link, line, character) => {
            if (link.startsWith("http") || link.startsWith("file:"))
                Qt.openUrlExternally(link)
            else if (line > 0)
                window.note(qsTr("open %1 at line %2").arg(link).arg(line))
            else
                window.note(qsTr("open %1").arg(link))
        }

        // What a screen reader is given. The library provides the text; a
        // role and a name are the application's to set.
        Accessible.role: Accessible.Terminal
        Accessible.name: qsTr("terminal")
        Accessible.description: terminal.accessibleText
    }

    Rectangle {
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: 12
        width: scrolledBack.implicitWidth + 16
        height: scrolledBack.implicitHeight + 8
        radius: 4
        visible: terminal.scrollOffset > 0
        color: colours.selectionBackground

        Label {
            id: scrolledBack
            anchors.centerIn: parent
            color: colours.foreground
            text: qsTr("%1 lines back").arg(terminal.scrollOffset)
        }
    }

    footer: Column {
        ToolBar {
            id: findBar
            visible: false
            width: parent.width

            Row {
                anchors.fill: parent
                anchors.margins: 4
                spacing: 8

                TextField {
                    id: findField
                    width: 260
                    placeholderText: qsTr("find in the scrollback")
                    onTextChanged: search.query = text
                    onAccepted: search.findNext()
                    Keys.onEscapePressed: closeFind()
                }
                Label {
                    anchors.verticalCenter: parent.verticalCenter
                    text: search.matchCount > 0
                          ? qsTr("%1 of %2").arg(search.currentIndex + 1).arg(search.matchCount)
                          : (search.query === "" ? "" : qsTr("no matches"))
                }
                Button { text: qsTr("Previous"); onClicked: search.findPrevious() }
                Button { text: qsTr("Next"); onClicked: search.findNext() }
                Button { text: qsTr("Close"); onClicked: closeFind() }
            }
        }

        Label {
            id: status
            objectName: "status"
            visible: false
            width: parent.width
            padding: 6
            color: colours.foreground
            background: Rectangle { color: Qt.darker(colours.background, 1.4) }
        }
    }

    function closeFind() {
        findBar.visible = false
        search.clear()
        findField.text = ""
        terminal.forceActiveFocus()
    }

    Shortcut {
        sequence: "Ctrl+Shift+F"
        onActivated: {
            findBar.visible = true
            findField.forceActiveFocus()
            findField.selectAll()
        }
    }

    Shortcut {
        sequence: "Ctrl+Shift+N"
        onActivated: window.note(
            qsTr("Ctrl+Shift+N is reserved by this application, so the shell never saw it"))
    }
}
