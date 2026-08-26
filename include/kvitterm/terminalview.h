// The terminal as something to look at and type into.
//
// Draws the visible rows of a session's screen, and turns keyboard, mouse,
// clipboard and input-method events into what the child process expects. Only
// the rows on screen are drawn, from a cache of shaped glyph runs, and only
// the regions the emulator reported as damaged are repainted.
//
// Which keystrokes belong to the terminal and which belong to the application
// around it has no neutral answer, so it is the application's to make:
// `reservedShortcuts` names the ones this item must not consume.
#pragma once

#include <QtCore/QPoint>
#include <QtGui/QFont>
#include <QtQuick/QQuickPaintedItem>
#include <QtQml/qqmlregistration.h>

#include "kvitterm_global.h"
#include "shellintegration.h"
#include "terminalpalette.h"
#include "terminalsearch.h"
#include "terminalsession.h"

QT_BEGIN_NAMESPACE
class QTimer;
QT_END_NAMESPACE

namespace kvitterm {

class KVITTERM_EXPORT TerminalView : public QQuickPaintedItem
{
    Q_OBJECT
    QML_ELEMENT

    Q_PROPERTY(kvitterm::TerminalSession *session READ session WRITE setSession NOTIFY sessionChanged)
    Q_PROPERTY(kvitterm::TerminalPalette *palette READ palette WRITE setPalette NOTIFY paletteChanged)
    Q_PROPERTY(QFont font READ font WRITE setFont NOTIFY fontChanged)
    // How far back the view is scrolled, in lines. Zero is the bottom, where
    // new output appears.
    Q_PROPERTY(int scrollOffset READ scrollOffset WRITE setScrollOffset NOTIFY scrollOffsetChanged)
    Q_PROPERTY(int columns READ columns NOTIFY gridChanged)
    Q_PROPERTY(int rows READ rows NOTIFY gridChanged)
    Q_PROPERTY(int scrollbackCount READ scrollbackCount NOTIFY scrollOffsetChanged)
    // Shortcuts this item must leave alone, as key sequences ("Ctrl+Shift+T").
    // They reach the application instead, and they take precedence over the
    // item's own copy, paste and scrolling shortcuts.
    Q_PROPERTY(QStringList reservedShortcuts READ reservedShortcuts WRITE setReservedShortcuts
                       NOTIFY reservedShortcutsChanged)
    Q_PROPERTY(bool hasSelection READ hasSelection NOTIFY selectionChanged)
    Q_PROPERTY(QString selectedText READ selectedText NOTIFY selectionChanged)
    // Matches drawn on top of the screen, and the current one scrolled to.
    Q_PROPERTY(kvitterm::TerminalSearch *search READ search WRITE setSearch NOTIFY searchChanged)
    // Optional. With it, the view can say which command the output at the top
    // of the window belongs to, which is what a sticky heading needs.
    Q_PROPERTY(kvitterm::ShellIntegration *shellIntegration READ shellIntegration
                       WRITE setShellIntegration NOTIFY shellIntegrationChanged)
    Q_PROPERTY(QString stickyCommand READ stickyCommand NOTIFY stickyCommandChanged)
    // The link under the pointer while a modifier is held, empty otherwise.
    Q_PROPERTY(QString hoveredLink READ hoveredLink NOTIFY hoveredLinkChanged)
    // The visible screen as plain text, for a screen reader. An application
    // binds Accessible.description to it and sets Accessible.role to Terminal.
    Q_PROPERTY(QString accessibleText READ accessibleText NOTIFY accessibleTextChanged)

public:
    explicit TerminalView(QQuickItem *parent = nullptr);
    ~TerminalView() override;

    TerminalSession *session() const;
    void setSession(TerminalSession *session);
    TerminalPalette *palette() const;
    void setPalette(TerminalPalette *palette);
    QFont font() const;
    void setFont(const QFont &font);
    int scrollOffset() const;
    void setScrollOffset(int offset);
    int columns() const;
    int rows() const;
    int scrollbackCount() const;
    QStringList reservedShortcuts() const;
    void setReservedShortcuts(const QStringList &shortcuts);
    bool hasSelection() const;
    QString selectedText() const;
    TerminalSearch *search() const;
    void setSearch(TerminalSearch *search);
    ShellIntegration *shellIntegration() const;
    void setShellIntegration(ShellIntegration *integration);
    QString stickyCommand() const;
    QString hoveredLink() const;
    QString accessibleText() const;

    Q_INVOKABLE void copy();
    Q_INVOKABLE void paste();
    Q_INVOKABLE void selectAll();
    Q_INVOKABLE void clearSelection();
    Q_INVOKABLE void scrollBy(int lines);
    Q_INVOKABLE void scrollToBottom();
    // Where a cell is on screen, for an application drawing something of its
    // own over the terminal.
    Q_INVOKABLE QRectF cellRect(int column, int row) const;

    void paint(QPainter *painter) override;

Q_SIGNALS:
    void sessionChanged();
    void paletteChanged();
    void fontChanged();
    void scrollOffsetChanged();
    void gridChanged();
    void reservedShortcutsChanged();
    void selectionChanged();
    void searchChanged();
    void shellIntegrationChanged();
    void stickyCommandChanged();
    void hoveredLinkChanged();
    void accessibleTextChanged();
    // A link the user activated, with the line and column that followed it
    // where there were any, and -1 where there were not. The view finds them;
    // what to do with one is the application's decision, since a terminal
    // library has no business choosing an editor.
    void linkActivated(const QString &link, int line, int character);

protected:
    void geometryChange(const QRectF &newGeometry, const QRectF &oldGeometry) override;
    void keyPressEvent(QKeyEvent *event) override;
    void mousePressEvent(QMouseEvent *event) override;
    void mouseMoveEvent(QMouseEvent *event) override;
    void mouseReleaseEvent(QMouseEvent *event) override;
    void mouseDoubleClickEvent(QMouseEvent *event) override;
    void hoverMoveEvent(QHoverEvent *event) override;
    void wheelEvent(QWheelEvent *event) override;
    void focusInEvent(QFocusEvent *event) override;
    void focusOutEvent(QFocusEvent *event) override;
    void inputMethodEvent(QInputMethodEvent *event) override;
    QVariant inputMethodQuery(Qt::InputMethodQuery query) const override;

private:
    class Private;
    Private *d;
};

} // namespace kvitterm
