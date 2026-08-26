// The colour scheme, as something QML can set piece by piece.
//
// The emulator keeps colours as the program named them — "the default
// foreground", "colour 4", or exact red-green-blue values — and resolves them
// only when drawing. Changing anything here therefore recolours what is
// already on the screen without the program inside it redrawing.
#pragma once

#include <QtCore/QObject>
#include <QtQml/qqmlregistration.h>

#include "kvitterm_global.h"
#include "palette.h"

namespace kvitterm {

class KVITTERM_EXPORT TerminalPalette : public QObject
{
    Q_OBJECT
    QML_ELEMENT

    Q_PROPERTY(QColor background READ background WRITE setBackground NOTIFY changed)
    Q_PROPERTY(QColor foreground READ foreground WRITE setForeground NOTIFY changed)
    Q_PROPERTY(QColor cursor READ cursor WRITE setCursor NOTIFY changed)
    Q_PROPERTY(QColor cursorText READ cursorText WRITE setCursorText NOTIFY changed)
    Q_PROPERTY(QColor selectionBackground READ selectionBackground WRITE setSelectionBackground
                       NOTIFY changed)
    Q_PROPERTY(QColor selectionForeground READ selectionForeground WRITE setSelectionForeground
                       NOTIFY changed)
    // The sixteen named colours, in the order every terminal numbers them:
    // eight normal, then the same eight bright.
    Q_PROPERTY(QVariantList ansiColors READ ansiColors WRITE setAnsiColors NOTIFY changed)

public:
    explicit TerminalPalette(QObject *parent = nullptr);

    QColor background() const;
    void setBackground(const QColor &color);
    QColor foreground() const;
    void setForeground(const QColor &color);
    QColor cursor() const;
    void setCursor(const QColor &color);
    QColor cursorText() const;
    void setCursorText(const QColor &color);
    QColor selectionBackground() const;
    void setSelectionBackground(const QColor &color);
    QColor selectionForeground() const;
    void setSelectionForeground(const QColor &color);
    QVariantList ansiColors() const;
    void setAnsiColors(const QVariantList &colors);

    const Palette &palette() const { return m_palette; }

Q_SIGNALS:
    void changed();

private:
    Palette m_palette;
};

} // namespace kvitterm
