// What in a line of output is worth clicking, and what is not. The second
// half matters more than the first: a detector that turns half of ordinary
// prose into links is worse than none.
#include <QtTest/QtTest>

#include "kvitterm/links.h"

using namespace kvitterm;

class TestLinks : public QObject
{
    Q_OBJECT

private Q_SLOTS:
    void webAddressesAreFound()
    {
        const QList<Link> links = findLinks(QStringLiteral("see https://example.invalid/a/b?c=1 now"));
        QCOMPARE(links.size(), 1);
        QCOMPARE(links.at(0).kind, Link::Url);
        QCOMPARE(links.at(0).text, QStringLiteral("https://example.invalid/a/b?c=1"));
        QCOMPARE(links.at(0).column, 4);
    }

    void punctuationAfterAnAddressIsNotPartOfIt()
    {
        const QList<Link> links = findLinks(QStringLiteral("read https://example.invalid/page."));
        QCOMPARE(links.size(), 1);
        QCOMPARE(links.at(0).text, QStringLiteral("https://example.invalid/page"));
    }

    void aClosingBracketThatBelongsToTheAddressIsKept()
    {
        const QList<Link> links =
            findLinks(QStringLiteral("https://example.invalid/wiki/Terminal_(disambiguation)"));
        QCOMPARE(links.size(), 1);
        QVERIFY(links.at(0).text.endsWith(QLatin1Char(')')));
    }

    void aPathWithALineNumberIsFound()
    {
        // The shape every compiler and test runner prints, which is the whole
        // reason for detecting paths at all.
        const QList<Link> links =
            findLinks(QStringLiteral("src/core/screen.cpp:42:7: error: no such thing"));
        QVERIFY(!links.isEmpty());
        const Link &link = links.at(0);
        QCOMPARE(link.kind, Link::Path);
        QCOMPARE(link.text, QStringLiteral("src/core/screen.cpp:42:7"));
        QCOMPARE(link.line, 42);
        QCOMPARE(link.character, 7);
    }

    void pathsInSeveralShapes()
    {
        QCOMPARE(findLinks(QStringLiteral("./build/log.txt")).size(), 1);
        QCOMPARE(findLinks(QStringLiteral("~/notes/todo.md")).size(), 1);
        QCOMPARE(findLinks(QStringLiteral("/usr/share/doc/README")).size(), 1);
        QCOMPARE(findLinks(QStringLiteral("tests/unit/test_links.cpp:9")).at(0).line, 9);
    }

    void aPathInsideAnAddressIsNotReportedTwice()
    {
        const QList<Link> links = findLinks(QStringLiteral("https://example.invalid/a/b/c.txt"));
        QCOMPARE(links.size(), 1);
        QCOMPARE(links.at(0).kind, Link::Url);
    }

    void ordinaryProseIsLeftAlone()
    {
        QVERIFY(findLinks(QStringLiteral("this line has no links in it at all")).isEmpty());
        QVERIFY(findLinks(QStringLiteral("total 0")).isEmpty());
        QVERIFY(findLinks(QStringLiteral("100% tests passed, 0 tests failed")).isEmpty());
        QVERIFY(findLinks(QStringLiteral("Compiling 12/34")).isEmpty());
    }

    void linksAreReportedInTheOrderTheyAppear()
    {
        const QList<Link> links =
            findLinks(QStringLiteral("see src/a.cpp:1 and https://example.invalid/x"));
        QCOMPARE(links.size(), 2);
        QVERIFY(links.at(0).column < links.at(1).column);
        QCOMPARE(links.at(0).kind, Link::Path);
        QCOMPARE(links.at(1).kind, Link::Url);
    }

    void aLineOfCellsCanBeSearchedDirectly()
    {
        Line line;
        for (const QChar character : QStringLiteral("open /tmp/x.log")) {
            Cell cell;
            cell.ch = character.unicode();
            line.cells.append(cell);
        }
        const QList<Link> links = findLinks(line);
        QCOMPARE(links.size(), 1);
        QCOMPARE(links.at(0).text, QStringLiteral("/tmp/x.log"));
    }
};

QTEST_APPLESS_MAIN(TestLinks)
#include "test_links.moc"
