// What the view puts on the screen, read back from an image.
//
// The rest of the suite asserts on the model behind the item, which is the
// right place for nearly everything. A grid is not in the model, though: the
// screen holds "M M M" whatever the drawing does with it, so a line drawn
// with its spaces squeezed out — the words closed up, every column after the
// first meaning nothing — passes every assertion that reads the model back.
// This file renders the item and looks at the pixels instead.
#include <QtGui/QFontDatabase>
#include <QtGui/QFontInfo>
#include <QtGui/QImage>
#include <QtGui/QPainter>
#include <QtTest/QtTest>

#include "kvitterm/screen.h"
#include "kvitterm/terminalsession.h"
#include "kvitterm/terminalview.h"

using namespace kvitterm;

namespace {

QImage render(TerminalView &view)
{
    QImage image(int(view.width()), int(view.height()), QImage::Format_ARGB32_Premultiplied);
    image.fill(Qt::transparent);
    QPainter painter(&image);
    view.paint(&painter);
    painter.end();
    return image;
}

// How many pixels of one region the view drew anything in. What counts as
// nothing is taken from a corner of the image rather than from the palette,
// so that this means "differs from the rest of the screen" whatever colours
// are in use.
int inkIn(const QImage &image, const QRectF &region)
{
    const QRgb background = image.pixel(image.width() - 1, image.height() - 1);
    int count = 0;
    for (int y = int(region.top()); y < int(region.bottom()) && y < image.height(); ++y) {
        for (int x = int(region.left()); x < int(region.right()) && x < image.width(); ++x) {
            if (image.pixel(x, y) != background)
                ++count;
        }
    }
    return count;
}

} // namespace

class TestRendering : public QObject
{
    Q_OBJECT

private Q_SLOTS:
    void unknownFamilyFallsBackToAFixedWidthFont();
    void everyCellKeepsItsColumn_data();
    void everyCellKeepsItsColumn();
};

void TestRendering::unknownFamilyFallsBackToAFixedWidthFont()
{
    // An application names a font it likes without being able to know the
    // machine has it, and "monospace" itself is a fontconfig alias that
    // exists on Linux and nowhere else. What a missing family falls back to
    // is therefore the common case rather than the odd one, and it has to be
    // another fixed-width font: the interface font is proportional, and a
    // terminal drawn in one has no grid.
    TerminalView view;
    view.setFont(QFont(QStringLiteral("no such family, and none like it")));
    QVERIFY(QFontInfo(view.font()).fixedPitch());
}

void TestRendering::everyCellKeepsItsColumn_data()
{
    QTest::addColumn<QFont>("font");
    QTest::newRow("the fixed-width font a terminal is given by default")
            << QFontDatabase::systemFont(QFontDatabase::FixedFont);
    // Not a mistake to test: an application is free to set one, and until the
    // renderer placed cells itself this is the case that lost the spaces.
    QTest::newRow("a proportional font the application asked for")
            << QFontDatabase::systemFont(QFontDatabase::GeneralFont);
}

void TestRendering::everyCellKeepsItsColumn()
{
    QFETCH(QFont, font);

    TerminalSession session;
    session.setAutoStart(false);
    TerminalView view;
    view.setFont(font);
    view.setSession(&session);
    view.setWidth(600);
    view.setHeight(200);

    // Letters in columns 0, 2 and 4 with a space between each pair, all of it
    // one run of identical styling — which is what is drawn in one call, and
    // so where a space that advances by less than a cell does its damage. The
    // newline moves the cursor off the row, since the cursor is drawn too.
    session.screen()->feed("M M M\r\n");
    const QImage image = render(view);

    for (int column = 0; column <= 4; column += 2) {
        const QRectF cell = view.cellRect(column, 0);
        QVERIFY2(inkIn(image, cell) > 0,
                 qPrintable(QStringLiteral("nothing drawn in column %1").arg(column)));
    }
    for (int column = 1; column <= 3; column += 2) {
        // The middle half of the cell, so that a neighbouring glyph's
        // antialiasing cannot be mistaken for a letter in the wrong column.
        const QRectF cell = view.cellRect(column, 0);
        const QRectF middle = cell.adjusted(cell.width() / 4, 0, -cell.width() / 4, 0);
        QVERIFY2(inkIn(image, middle) == 0,
                 qPrintable(QStringLiteral("column %1 holds a space and was drawn in")
                                    .arg(column)));
    }
}

QTEST_MAIN(TestRendering)
#include "test_rendering.moc"
