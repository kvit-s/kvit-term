// The emulator: a terminal byte stream in, a screen of cells out.
//
// This is the class that knows what `\x1b[31m` means. It holds the visible
// grid, the scrollback above it, the cursor, the title, and the modes a
// program can select that change how keys are encoded. It has no window, no
// child process and no drawing code, which is why the whole of its behaviour
// can be proved by feeding it recorded bytes and reading the grid back.
//
// Rows are numbered from the top of the visible screen: row 0 is the first
// visible line, row -1 is the line most recently scrolled off, and row
// -scrollbackCount() is the oldest line still kept.
#pragma once

#include <QtCore/QObject>
#include <QtCore/QPoint>
#include <QtCore/QRect>

#include "cell.h"
#include "kvitterm_global.h"

namespace kvitterm {

enum class CursorShape { Block, Underline, Bar };

// What a program has asked to be told about the mouse. A terminal reports
// nothing until a program asks, which is why selecting text with the mouse
// works in a shell and stops working inside a full-screen editor.
enum class MouseTracking { None, Click, Drag, Motion };

enum class MouseButton { None = 0, Left = 1, Middle = 2, Right = 3, WheelUp = 4, WheelDown = 5 };

class ScreenPrivate;

class KVITTERM_EXPORT Screen : public QObject
{
    Q_OBJECT

public:
    explicit Screen(int columns = 80, int rows = 24, QObject *parent = nullptr);
    ~Screen() override;

    // ── Feeding it
    void feed(const QByteArray &bytes);
    void reset(bool hard = true);

    // ── Shape
    int columns() const;
    int rows() const;
    void setSize(int columns, int rows);
    int scrollbackCount() const;
    int scrollbackLimit() const;
    void setScrollbackLimit(int lines);
    void clearScrollback();

    // ── Contents
    Line line(int row) const;
    Cell cell(int row, int column) const;
    QString text(int fromRow, int toRow) const;
    // Text of a rectangular or linear range, for a selection. A linear
    // selection runs from one point to another through the ends of lines; a
    // block selection takes the same columns out of each row.
    QString textInRange(const QPoint &start, const QPoint &end, bool block = false) const;

    // ── State a view has to draw
    QPoint cursor() const;              // x is the column, y the row
    bool cursorVisible() const;
    bool cursorBlinks() const;
    CursorShape cursorShape() const;
    QString title() const;
    bool alternateScreen() const;
    bool reverseVideo() const;
    MouseTracking mouseTracking() const;
    bool focusReporting() const;
    bool bracketedPaste() const;

    // ── Input, which becomes bytes for the child through writeRequested
    void keyPress(int qtKey, Qt::KeyboardModifiers modifiers, const QString &text);
    void sendText(const QString &text);
    void paste(const QString &text);
    void mouseMove(int row, int column, Qt::KeyboardModifiers modifiers);
    void mouseButton(MouseButton button, bool pressed, Qt::KeyboardModifiers modifiers);
    void setFocused(bool focused);

Q_SIGNALS:
    // In cell coordinates, with the row relative to the top of the visible
    // screen. A view coalesces these into one repaint per frame.
    void damaged(const QRect &region);
    // Lines have moved off the top of the screen into the scrollback.
    void scrolled(int lines);
    void scrollbackCleared();
    void sizeChanged(int columns, int rows);
    void cursorMoved(const QPoint &position);
    void titleChanged(const QString &title);
    void bell();
    void alternateScreenChanged(bool active);
    void mouseTrackingChanged(MouseTracking tracking);
    // Bytes the child must receive: a key that was pressed, a mouse report, or
    // an answer to a question the program asked the terminal.
    void writeRequested(const QByteArray &bytes);
    // An operating-system command this class does not itself act on. The
    // shell-integration markers arrive here; see shellmarks.h.
    void osc(int command, const QByteArray &payload);

private:
    friend class ScreenPrivate;
    ScreenPrivate *d;
};

} // namespace kvitterm
