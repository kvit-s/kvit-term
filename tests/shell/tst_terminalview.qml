import QtQuick
import QtTest
import KvitTerm

// The view, driven as an application would drive it: a real item in a real
// window, asserting on the model behind it rather than on pixels.
Item {
    id: root
    width: 480
    height: 240

    TerminalPalette {
        id: colours
    }

    TerminalSession {
        id: session
        objectName: "session"
        autoStart: false
        program: testStubPath          // set from C++ as a context property
    }

    TerminalView {
        id: view
        objectName: "view"
        anchors.fill: parent
        session: session
        palette: colours
        font.family: "monospace"
        font.pointSize: 10
        focus: true
    }

    TestCase {
        name: "TerminalView"
        when: windowShown

        // Each case gets a session of its own in all but name: the previous
        // child is ended and the screen wiped, so that nothing a case sees
        // was left behind by the one before it. The echo scenario in
        // particular waits for input and would otherwise still be running.
        function init() {
            session.close()
            tryVerify(function () { return !session.running })
            session.clear()
            view.clearSelection()
            view.scrollToBottom()
        }

        function test_theGridFollowsTheItemSize() {
            // The child is told a size derived from the item's geometry and
            // the font, which is what makes a full-screen program fit.
            verify(view.columns > 10)
            verify(view.rows > 3)
            compare(session.columns, view.columns)
            compare(session.rows, view.rows)
        }

        function test_aProgramsOutputReachesTheScreen() {
            session.arguments = ["scenario", "colour"]
            verify(session.start())
            tryVerify(function () { return session.screenText().indexOf("truecolour") >= 0 })
        }

        function test_whatIsTypedReachesTheChild() {
            session.arguments = ["echo"]
            verify(session.start())
            view.forceActiveFocus()
            keyClick(Qt.Key_H)
            keyClick(Qt.Key_I)
            keyClick(Qt.Key_Return)
            tryVerify(function () { return session.screenText().indexOf("echo:hi") >= 0 })
        }

        function test_selectionCanBeReadBack() {
            session.arguments = ["scenario", "links"]
            verify(session.start())
            tryVerify(function () { return session.screenText().indexOf("example") >= 0 })
            view.selectAll()
            verify(view.hasSelection)
            verify(view.selectedText.indexOf("example.invalid") >= 0)
            view.clearSelection()
            verify(!view.hasSelection)
        }

        function test_scrollingBackAndReturning() {
            session.arguments = ["scenario", "scroll"]
            verify(session.start())
            tryVerify(function () { return view.scrollbackCount > 5 })
            view.scrollBy(5)
            compare(view.scrollOffset, 5)
            // Typing returns to where new output appears, which is what makes
            // scrolling back safe.
            view.forceActiveFocus()
            keyClick(Qt.Key_X)
            compare(view.scrollOffset, 0)
        }

        function test_aReservedShortcutIsNotConsumed() {
            // The application asked for this one, so the item must not treat
            // it as its own and must not send it to the child either.
            session.arguments = ["echo"]
            verify(session.start())
            view.reservedShortcuts = ["Ctrl+Shift+C"]
            view.forceActiveFocus()
            const before = session.screenText()
            keyClick(Qt.Key_C, Qt.ControlModifier | Qt.ShiftModifier)
            compare(session.screenText(), before)
        }
    }
}
