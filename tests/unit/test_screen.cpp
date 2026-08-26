// The emulator, proved by feeding it recorded bytes and reading the screen
// back. No window and no child process are involved, which is what makes this
// the suite that grows: every program that turns out to emit something
// unexpected becomes another case here.
#include <QtCore/QCoreApplication>
#include <QtTest/QtTest>

#include "kvitterm/screen.h"
#include "kvitterm/screenexport.h"

using namespace kvitterm;

class TestScreen : public QObject
{
    Q_OBJECT

private:
    static QString rowText(const Screen &screen, int row) { return screen.line(row).text(); }

private Q_SLOTS:
    void plainTextLandsOnTheScreen()
    {
        Screen screen(20, 4);
        screen.feed("hello\r\nworld");
        QCOMPARE(rowText(screen, 0), QStringLiteral("hello"));
        QCOMPARE(rowText(screen, 1), QStringLiteral("world"));
        QCOMPARE(screen.cursor(), QPoint(5, 1));
    }

    void aCarriageReturnRedrawsTheLine()
    {
        // What a progress bar does, and what a text view showing raw output
        // gets wrong: the later text replaces the earlier text rather than
        // appearing after it.
        Screen screen(20, 2);
        screen.feed("working: 0%\rworking: 50%\rdone        ");
        // Trailing blanks are the width of the window rather than output, so
        // the line reads as what the program last drew.
        QCOMPARE(rowText(screen, 0), QStringLiteral("done"));
    }

    void coloursAndAttributesAreKept()
    {
        Screen screen(40, 2);
        screen.feed("\x1b[31mR\x1b[0m\x1b[1;32mG\x1b[0m\x1b[38;5;208mI\x1b[0m"
                    "\x1b[38;2;10;20;30mT\x1b[0m\x1b[4mU\x1b[24m\x1b[7mV\x1b[27m");

        const Cell red = screen.cell(0, 0);
        QCOMPARE(red.style.foreground.kind, Color::Indexed);
        QCOMPARE(int(red.style.foreground.index), 1);

        const Cell green = screen.cell(0, 1);
        QVERIFY(green.style.bold);
        QCOMPARE(int(green.style.foreground.index), 2);

        const Cell indexed = screen.cell(0, 2);
        QCOMPARE(indexed.style.foreground.kind, Color::Indexed);
        QCOMPARE(int(indexed.style.foreground.index), 208);

        const Cell truecolour = screen.cell(0, 3);
        QCOMPARE(truecolour.style.foreground.kind, Color::Rgb);
        QCOMPARE(int(truecolour.style.foreground.red), 10);
        QCOMPARE(int(truecolour.style.foreground.green), 20);
        QCOMPARE(int(truecolour.style.foreground.blue), 30);

        QCOMPARE(screen.cell(0, 4).style.underline, Underline::Single);
        QVERIFY(screen.cell(0, 5).style.reverse);
        // The colour is kept as the program named it rather than resolved to
        // pixels, which is what lets a colour scheme change without the
        // program redrawing.
        QCOMPARE(screen.cell(0, 6).style.foreground.kind, Color::Default);
    }

    void cursorMovementAndErasure()
    {
        Screen screen(10, 3);
        screen.feed("abcdefghij");
        screen.feed("\x1b[1;1H");        // home
        QCOMPARE(screen.cursor(), QPoint(0, 0));
        screen.feed("\x1b[2;3H");        // row 2, column 3
        QCOMPARE(screen.cursor(), QPoint(2, 1));
        screen.feed("\x1b[1;5H\x1b[K");  // erase to the end of the line
        QCOMPARE(rowText(screen, 0), QStringLiteral("abcd"));
        screen.feed("\x1b[2J");          // erase the screen
        QVERIFY(screen.line(0).isBlank());
    }

    void theAlternateScreenLeavesTheScrollbackAlone()
    {
        // A full-screen program runs on a second screen and gives the first
        // one back untouched when it exits, which is why quitting an editor
        // returns the shell's output rather than the editor's.
        Screen screen(20, 3);
        screen.feed("before\r\n");
        QVERIFY(!screen.alternateScreen());
        screen.feed("\x1b[?1049h");
        QVERIFY(screen.alternateScreen());
        screen.feed("\x1b[2J\x1b[Hinside");
        QCOMPARE(rowText(screen, 0), QStringLiteral("inside"));
        const int scrollbackDuring = screen.scrollbackCount();
        screen.feed("\r\n\r\n\r\nmore\r\n\r\n");
        QCOMPARE(screen.scrollbackCount(), scrollbackDuring);   // nothing was stored
        screen.feed("\x1b[?1049l");
        QVERIFY(!screen.alternateScreen());
        QCOMPARE(rowText(screen, 0), QStringLiteral("before"));
    }

    void linesScrollIntoTheScrollback()
    {
        Screen screen(20, 3);
        for (int line = 1; line <= 10; ++line)
            screen.feed(QStringLiteral("line %1\r\n").arg(line).toUtf8());
        // The tenth newline left the cursor on an empty eleventh line, so the
        // screen holds lines 9 and 10 and a blank, and everything before them
        // is stored.
        QCOMPARE(screen.scrollbackCount(), 8);
        QCOMPARE(rowText(screen, -1), QStringLiteral("line 8"));
        QCOMPARE(rowText(screen, -8), QStringLiteral("line 1"));
        QCOMPARE(rowText(screen, 0), QStringLiteral("line 9"));
        QCOMPARE(rowText(screen, 1), QStringLiteral("line 10"));
    }

    void theScrollbackLimitIsHonoured()
    {
        Screen screen(20, 3);
        screen.setScrollbackLimit(5);
        for (int line = 1; line <= 40; ++line)
            screen.feed(QStringLiteral("line %1\r\n").arg(line).toUtf8());
        QCOMPARE(screen.scrollbackCount(), 5);
        QCOMPARE(rowText(screen, -1), QStringLiteral("line 38"));
        QCOMPARE(rowText(screen, -5), QStringLiteral("line 34"));
    }

    void doubleWidthCharactersOccupyTwoCells()
    {
        Screen screen(10, 2);
        screen.feed(QStringLiteral("[日本]").toUtf8());
        QCOMPARE(screen.cell(0, 0).text(), QStringLiteral("["));
        QCOMPARE(screen.cell(0, 1).text(), QStringLiteral("日"));
        QCOMPARE(int(screen.cell(0, 1).width), 2);
        QCOMPARE(int(screen.cell(0, 2).width), 0);       // the right-hand half
        QVERIFY(screen.cell(0, 2).text().isEmpty());
        QCOMPARE(screen.cell(0, 5).text(), QStringLiteral("]"));
        QCOMPARE(rowText(screen, 0), QStringLiteral("[日本]"));
    }

    void combiningMarksStayInOneCell()
    {
        Screen screen(10, 2);
        screen.feed(QStringLiteral("éx").toUtf8());   // e, combining acute, x
        QCOMPARE(screen.cell(0, 0).text(), QStringLiteral("é"));
        QCOMPARE(screen.cell(0, 1).text(), QStringLiteral("x"));
    }

    void theTitleIsReported()
    {
        Screen screen(20, 2);
        QSignalSpy titles(&screen, &Screen::titleChanged);
        screen.feed("\x1b]0;a title\x1b\\");
        QCOMPARE(screen.title(), QStringLiteral("a title"));
        QCOMPARE(titles.count(), 1);
    }

    void theBellIsReported()
    {
        Screen screen(20, 2);
        QSignalSpy bells(&screen, &Screen::bell);
        screen.feed("\a");
        QCOMPARE(bells.count(), 1);
    }

    void unhandledOperatingSystemCommandsReachTheCaller()
    {
        // This is the seam the shell-integration layer is built on: libvterm
        // acts on the commands it knows and hands the rest here.
        Screen screen(20, 2);
        QSignalSpy commands(&screen, &Screen::osc);
        screen.feed("\x1b]133;A\x1b\\\x1b]7;file://host/tmp\x1b\\");
        QCOMPARE(commands.count(), 2);
        QCOMPARE(commands.at(0).at(0).toInt(), 133);
        QCOMPARE(commands.at(0).at(1).toByteArray(), QByteArray("A"));
        QCOMPARE(commands.at(1).at(0).toInt(), 7);
        QCOMPARE(commands.at(1).at(1).toByteArray(), QByteArray("file://host/tmp"));
    }

    void keysBecomeTheBytesATerminalSends()
    {
        Screen screen(20, 4);
        QSignalSpy written(&screen, &Screen::writeRequested);
        const auto lastWrite = [&written] { return written.last().at(0).toByteArray(); };

        screen.keyPress(Qt::Key_A, Qt::NoModifier, QStringLiteral("a"));
        QCOMPARE(lastWrite(), QByteArray("a"));

        screen.keyPress(Qt::Key_C, Qt::ControlModifier, QStringLiteral("\x03"));
        QCOMPARE(lastWrite(), QByteArray("\x03"));       // interrupt

        screen.keyPress(Qt::Key_Return, Qt::NoModifier, QStringLiteral("\r"));
        QCOMPARE(lastWrite(), QByteArray("\r"));

        screen.keyPress(Qt::Key_Tab, Qt::NoModifier, QStringLiteral("\t"));
        QCOMPARE(lastWrite(), QByteArray("\t"));

        screen.keyPress(Qt::Key_Up, Qt::NoModifier, QString());
        QCOMPARE(lastWrite(), QByteArray("\x1b[A"));

        // The same key sends something different once a program has asked for
        // application cursor keys, which is why an editor's arrow keys keep
        // working when the shell's history does too.
        screen.feed("\x1b[?1h");
        screen.keyPress(Qt::Key_Up, Qt::NoModifier, QString());
        QCOMPARE(lastWrite(), QByteArray("\x1bOA"));
        screen.feed("\x1b[?1l");

        screen.keyPress(Qt::Key_F1, Qt::NoModifier, QString());
        QCOMPARE(lastWrite(), QByteArray("\x1bOP"));

        screen.keyPress(Qt::Key_Backspace, Qt::NoModifier, QString());
        QCOMPARE(lastWrite(), QByteArray("\x7f"));
    }

    void pasteIsMarkedOnlyWhenTheProgramAsksForIt()
    {
        Screen screen(20, 4);
        QSignalSpy written(&screen, &Screen::writeRequested);

        screen.paste(QStringLiteral("one\ntwo"));
        QByteArray plain;
        for (const auto &call : written)
            plain += call.at(0).toByteArray();
        QCOMPARE(plain, QByteArray("one\rtwo"));

        written.clear();
        screen.feed("\x1b[?2004h");            // the program asks for bracketed paste
        screen.paste(QStringLiteral("one"));
        QByteArray bracketed;
        for (const auto &call : written)
            bracketed += call.at(0).toByteArray();
        QCOMPARE(bracketed, QByteArray("\x1b[200~one\x1b[201~"));
    }

    void theMouseIsReportedOnlyWhenTheProgramAsksForIt()
    {
        Screen screen(20, 4);
        QSignalSpy written(&screen, &Screen::writeRequested);
        QCOMPARE(screen.mouseTracking(), MouseTracking::None);
        screen.mouseButton(MouseButton::Left, true, Qt::NoModifier);
        screen.mouseButton(MouseButton::Left, false, Qt::NoModifier);
        QCOMPARE(written.count(), 0);

        screen.feed("\x1b[?1000h");
        QCOMPARE(screen.mouseTracking(), MouseTracking::Click);
        screen.mouseMove(2, 3, Qt::NoModifier);
        screen.mouseButton(MouseButton::Left, true, Qt::NoModifier);
        QCOMPARE(written.count(), 1);
        // The X10 form: a control sequence carrying the button, then the
        // column and row biased by 33 so that they are printable.
        QCOMPARE(written.last().at(0).toByteArray(), QByteArray("\x1b[M\x20\x24\x23"));
    }

    void theTerminalAnswersQuestionsAboutItself()
    {
        // A program asks where the cursor is; the terminal replies. Without
        // this, tools that measure the window before drawing hang.
        Screen screen(20, 4);
        QSignalSpy written(&screen, &Screen::writeRequested);
        screen.feed("\x1b[3;7H\x1b[6n");
        QVERIFY(written.count() > 0);
        QCOMPARE(written.last().at(0).toByteArray(), QByteArray("\x1b[3;7R"));
    }

    void aWrappedLineIsMarkedAsOne()
    {
        // The flag that says a line began as the overflow of the one above.
        // libvterm keeps it for the visible screen; keeping it for a line that
        // has scrolled away is this library's own bookkeeping.
        Screen screen(10, 4);
        screen.feed("0123456789ABCDE\r\n");
        QVERIFY(!screen.line(0).continuation);
        QVERIFY(screen.line(1).continuation);

        for (int line = 0; line < 12; ++line)
            screen.feed("x\r\n");

        // Find the stored halves of that first long line and check the second
        // half still knows it was a continuation.
        int foundFirstHalf = -1;
        for (int row = -screen.scrollbackCount(); row < 0; ++row) {
            if (screen.line(row).text() == QStringLiteral("0123456789"))
                foundFirstHalf = row;
        }
        QVERIFY2(foundFirstHalf != -1, "the wrapped line was not found in the scrollback");
        QVERIFY(!screen.line(foundFirstHalf).continuation);
        QCOMPARE(screen.line(foundFirstHalf + 1).text(), QStringLiteral("ABCDE"));
        QVERIFY(screen.line(foundFirstHalf + 1).continuation);
    }

    void resizingRewrapsTheVisibleScreen()
    {
        Screen screen(10, 4);
        screen.feed("0123456789ABCDEFGHIJ");
        QCOMPARE(rowText(screen, 0), QStringLiteral("0123456789"));
        QCOMPARE(rowText(screen, 1), QStringLiteral("ABCDEFGHIJ"));
        screen.setSize(20, 4);
        QCOMPARE(rowText(screen, 0), QStringLiteral("0123456789ABCDEFGHIJ"));
    }

    void resizingRewrapsTheScrollbackToo()
    {
        // libvterm re-wraps the visible screen and leaves the history at
        // whatever line breaks it had, which is what Neovim's terminal does.
        // This library re-wraps the history as well, so that widening the
        // window makes old output readable rather than ragged.
        Screen screen(10, 4);
        screen.feed("0123456789ABCDEFGHIJ\r\n");
        for (int line = 0; line < 12; ++line)
            screen.feed("x\r\n");

        screen.setSize(20, 4);
        int joined = 0;
        for (int row = -screen.scrollbackCount(); row < 0; ++row) {
            if (screen.line(row).text() == QStringLiteral("0123456789ABCDEFGHIJ"))
                ++joined;
        }
        QCOMPARE(joined, 1);

        screen.setSize(5, 4);
        QStringList pieces;
        for (int row = -screen.scrollbackCount(); row < 0; ++row) {
            const QString text = screen.line(row).text();
            if (text.startsWith(QLatin1Char('0')) || text.startsWith(QLatin1Char('5'))
                || text.startsWith(QLatin1Char('A')) || text.startsWith(QLatin1Char('F')))
                pieces << text;
        }
        QCOMPARE(pieces, QStringList({QStringLiteral("01234"), QStringLiteral("56789"),
                                      QStringLiteral("ABCDE"), QStringLiteral("FGHIJ")}));
    }

    void textIsReadBackAcrossLinesAndInBlocks()
    {
        Screen screen(20, 4);
        screen.feed("alpha beta\r\ngamma delta\r\n");
        QCOMPARE(screen.textInRange(QPoint(6, 0), QPoint(4, 1)),
                 QStringLiteral("beta\ngamma"));
        QCOMPARE(screen.textInRange(QPoint(0, 0), QPoint(4, 1), true),
                 QStringLiteral("alpha\ngamma"));
    }

    void whatIsOnTheScreenComesOutAsStyledText()
    {
        // The path a build log takes when an application shows it in an
        // ordinary text view rather than in a terminal.
        Screen screen(20, 3);
        screen.feed("plain \x1b[31mred\x1b[0m \x1b[1mbold\x1b[0m\r\n<escaped>");

        const QString plain = exportPlainText(screen, 0, 1);
        QCOMPARE(plain, QStringLiteral("plain red bold\n<escaped>"));

        const QString html = exportHtml(screen, 0, 1);
        QVERIFY(html.startsWith(QStringLiteral("<pre")));
        QVERIFY(html.contains(QStringLiteral("&lt;escaped&gt;")));
        QVERIFY(html.contains(QStringLiteral("font-weight:bold")));
        // The red is the palette's red rather than a colour the program chose,
        // since the program named an index and the palette owns what it means.
        QVERIFY(html.contains(Palette().ansi[1].name()));
    }

    void aWrappedLineIsCopiedAsOneLine()
    {
        Screen screen(10, 4);
        screen.feed("0123456789ABCDE\r\n");
        QCOMPARE(screen.text(0, 1), QStringLiteral("0123456789ABCDE"));
    }
};

QTEST_MAIN(TestScreen)
#include "test_screen.moc"
