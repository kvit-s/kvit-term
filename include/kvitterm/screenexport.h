// Screen contents as styled text, for an application that has no terminal in
// its interface.
//
// A build log is the case this exists for. A program run under a
// pseudo-terminal emits colour and redraws its progress line with carriage
// returns; feeding that through `Screen` and exporting the result gives an
// ordinary text view something it can display, without a renderer, a panel,
// or any of the input handling a terminal needs.
#pragma once

#include <QtCore/QString>

#include "kvitterm_global.h"
#include "palette.h"

namespace kvitterm {

class Screen;

// Rows are numbered as in Screen: 0 is the top of the visible screen and
// negative rows are the scrollback. Passing `screen.scrollbackCount()`
// negated as the first row and `screen.rows() - 1` as the last gives
// everything the screen holds.
KVITTERM_EXPORT QString exportPlainText(const Screen &screen, int firstRow, int lastRow);

// A <pre> block with one <span> per run of identical styling. Self-contained:
// the colours are inline, so it can be handed to any rich-text view without a
// stylesheet.
KVITTERM_EXPORT QString exportHtml(const Screen &screen, int firstRow, int lastRow,
                                   const Palette &palette = Palette());

} // namespace kvitterm
