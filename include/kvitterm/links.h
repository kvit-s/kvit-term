// Finding the things in output that are worth clicking.
//
// Two kinds are worth the trouble, because they are what people actually
// reach for: a web address, and a file path with an optional line and column
// after it, which is the shape every compiler and test runner prints when
// something goes wrong.
//
// Detection happens here; what to do with a link is the application's. A
// terminal library has no business deciding that a path should open in an
// editor, and no way of knowing which one.
#pragma once

#include <QtCore/QList>
#include <QtCore/QString>

#include "cell.h"
#include "kvitterm_global.h"

namespace kvitterm {

struct KVITTERM_EXPORT Link
{
    enum Kind { Url, Path };

    Kind kind = Url;
    QString text;        // exactly the characters that matched
    int column = 0;      // where it starts on the line
    int length = 0;      // how many cells it covers
    int line = -1;       // the line number after a path, when there is one
    int character = -1;  // the column number after that, when there is one
};

// Links on one line. A line that was wrapped from the one above should be
// joined to it first, or an address split across the wrap will be missed.
KVITTERM_EXPORT QList<Link> findLinks(const QString &text);
KVITTERM_EXPORT QList<Link> findLinks(const Line &line);

} // namespace kvitterm
