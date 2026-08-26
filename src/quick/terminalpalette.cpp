#include "kvitterm/terminalpalette.h"

#include <QtCore/QVariant>

namespace kvitterm {

TerminalPalette::TerminalPalette(QObject *parent) : QObject(parent) {}

QColor TerminalPalette::background() const { return m_palette.background; }
QColor TerminalPalette::foreground() const { return m_palette.foreground; }
QColor TerminalPalette::cursor() const { return m_palette.cursor; }
QColor TerminalPalette::cursorText() const { return m_palette.cursorText; }
QColor TerminalPalette::selectionBackground() const { return m_palette.selectionBackground; }
QColor TerminalPalette::selectionForeground() const { return m_palette.selectionForeground; }

void TerminalPalette::setBackground(const QColor &color)
{
    if (m_palette.background == color)
        return;
    m_palette.background = color;
    Q_EMIT changed();
}

void TerminalPalette::setForeground(const QColor &color)
{
    if (m_palette.foreground == color)
        return;
    m_palette.foreground = color;
    Q_EMIT changed();
}

void TerminalPalette::setCursor(const QColor &color)
{
    if (m_palette.cursor == color)
        return;
    m_palette.cursor = color;
    Q_EMIT changed();
}

void TerminalPalette::setCursorText(const QColor &color)
{
    if (m_palette.cursorText == color)
        return;
    m_palette.cursorText = color;
    Q_EMIT changed();
}

void TerminalPalette::setSelectionBackground(const QColor &color)
{
    if (m_palette.selectionBackground == color)
        return;
    m_palette.selectionBackground = color;
    Q_EMIT changed();
}

void TerminalPalette::setSelectionForeground(const QColor &color)
{
    if (m_palette.selectionForeground == color)
        return;
    m_palette.selectionForeground = color;
    Q_EMIT changed();
}

QVariantList TerminalPalette::ansiColors() const
{
    QVariantList colors;
    colors.reserve(16);
    for (const QColor &color : m_palette.ansi)
        colors.append(color);
    return colors;
}

void TerminalPalette::setAnsiColors(const QVariantList &colors)
{
    // A shorter list leaves the rest of the scheme alone, which is what makes
    // it usable for overriding two or three colours from QML.
    const int count = qMin(16, int(colors.size()));
    bool anyChange = false;
    for (int index = 0; index < count; ++index) {
        const QColor color = colors.at(index).value<QColor>();
        if (!color.isValid() || m_palette.ansi[index] == color)
            continue;
        m_palette.ansi[index] = color;
        anyChange = true;
    }
    if (anyChange)
        Q_EMIT changed();
}

} // namespace kvitterm
