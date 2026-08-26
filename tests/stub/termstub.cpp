// termstub — the program the tests put on the other end of a pseudo-terminal.
//
// Every scenario a test needs is a named mode of this one program: whether it
// sees a terminal, what size it is told the terminal is, and each family of
// escape sequence the screen model has to interpret. Tests then depend on
// nothing that varies between machines — not a shell, not its version, not
// which tools happen to be installed.
//
// Deliberately free of Qt and of anything else that would need building for a
// platform before a test could run there.
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>

#ifdef _WIN32
#  include <io.h>
#  include <windows.h>
#  define isatty _isatty
#  define fileno _fileno
#else
#  include <csignal>
#  include <sys/ioctl.h>
#  include <unistd.h>
#endif

namespace {

void emitTerminalSize()
{
#ifdef _WIN32
    CONSOLE_SCREEN_BUFFER_INFO info;
    if (GetConsoleScreenBufferInfo(GetStdHandle(STD_OUTPUT_HANDLE), &info)) {
        // The window is what a program should size itself to; the buffer can
        // be taller. Both are reported, because which one a pseudoconsole
        // sets is exactly the sort of thing that differs.
        std::printf("size %dx%d buffer %dx%d\n", info.srWindow.Right - info.srWindow.Left + 1,
                    info.srWindow.Bottom - info.srWindow.Top + 1, int(info.dwSize.X),
                    int(info.dwSize.Y));
    } else {
        std::printf("size unknown (%lu)\n", (unsigned long) GetLastError());
    }
#else
    struct winsize size = {};
    if (ioctl(STDOUT_FILENO, TIOCGWINSZ, &size) == 0)
        std::printf("size %dx%d\n", int(size.ws_col), int(size.ws_row));
    else
        std::printf("size unknown\n");
#endif
    std::fflush(stdout);
}

#ifndef _WIN32
volatile sig_atomic_t g_resized = 0;
void onWindowChange(int) { g_resized = 1; }
#endif

// Every escape sequence below is written out in full rather than through a
// helper, so that a test failure points at the exact bytes that produced it.
void emitScenario(const std::string &name)
{
    if (name == "colour") {
        std::printf("\x1b[31mred\x1b[0m ");
        std::printf("\x1b[1;32mbold green\x1b[0m ");
        std::printf("\x1b[38;5;208m256-orange\x1b[0m ");
        std::printf("\x1b[38;2;120;180;240mtruecolour\x1b[0m ");
        std::printf("\x1b[4munderline\x1b[24m \x1b[7mreverse\x1b[27m\n");
    } else if (name == "altscreen") {
        std::printf("before\n");
        std::printf("\x1b[?1049h");          // switch to the alternate screen
        std::printf("\x1b[2J\x1b[Hinside the alternate screen");
        std::printf("\x1b[?1049l");          // and back, discarding it
        std::printf("after\n");
    } else if (name == "progress") {
        for (int percent = 0; percent <= 100; percent += 25)
            std::printf("\rworking: %d%%", percent);
        // Erase to the end of the line rather than padding with spaces, which
        // is what a tool that redraws a progress line actually does.
        std::printf("\rdone\x1b[K\n");
    } else if (name == "wide") {
        std::printf("[\xe6\x97\xa5\xe6\x9c\xac]\n");                 // two double-width ideographs
        std::printf("e\xcc\x81 = \xc3\xa9\n");                       // combining acute, then precomposed
    } else if (name == "scroll") {
        for (int line = 1; line <= 50; ++line)
            std::printf("line %d\n", line);
    } else if (name == "marks") {
        // The shell-integration markers: a prompt, the command typed at it,
        // its output, and the exit status it finished with.
        std::printf("\x1b]7;file://host/tmp\x1b\\");
        std::printf("\x1b]133;A\x1b\\$ \x1b]133;B\x1b\\ls -l\n");
        std::printf("\x1b]133;C\x1b\\total 0\nfile.txt\n");
        std::printf("\x1b]133;D;0\x1b\\");
    } else if (name == "title") {
        std::printf("\x1b]0;a title\x1b\\");
    } else if (name == "bell") {
        std::printf("\a");
    } else if (name == "links") {
        std::printf("see src/core/screen.cpp:42 and https://example.invalid/x\n");
    } else {
        std::printf("unknown scenario: %s\n", name.c_str());
    }
    std::fflush(stdout);
}

} // namespace

int main(int argc, char **argv)
{
    const std::string mode = argc > 1 ? argv[1] : "isatty";
    setvbuf(stdout, nullptr, _IONBF, 0);

    if (mode == "isatty") {
        std::printf("stdin %d stdout %d stderr %d\n", isatty(fileno(stdin)) ? 1 : 0,
                    isatty(fileno(stdout)) ? 1 : 0, isatty(fileno(stderr)) ? 1 : 0);
#ifdef _WIN32
        // What kind of thing standard output actually is, which is the first
        // question worth asking when a child does not believe it is on a
        // terminal: 2 is a character device, which is what a console is, and
        // 3 is a pipe, which is what it gets when the pseudoconsole was not
        // attached and it inherited the parent's handles instead.
        std::printf("stdout-type %lu\n",
                    (unsigned long) GetFileType(GetStdHandle(STD_OUTPUT_HANDLE)));
        // The handle values themselves, so that a child writing to the
        // parent's pipe can be told from one writing to its own terminal.
        std::printf("stdout-handle %p\n", (void *) GetStdHandle(STD_OUTPUT_HANDLE));
#endif
        std::fflush(stdout);
        return 0;
    }
    if (mode == "size") {
        emitTerminalSize();
        return 0;
    }
#ifndef _WIN32
    if (mode == "size-watch") {
        // Report the size now, then once more the first time the terminal is
        // resized under us. SIGWINCH is how that arrives.
        struct sigaction action = {};
        action.sa_handler = onWindowChange;
        sigaction(SIGWINCH, &action, nullptr);
        emitTerminalSize();
        while (!g_resized)
            pause();
        emitTerminalSize();
        return 0;
    }
#endif
    if (mode == "echo") {
        // Read a line at a time and repeat it back, which proves the write
        // path and the child's line discipline. "quit" ends it.
        char line[4096];
        while (std::fgets(line, sizeof(line), stdin)) {
            std::string text(line);
            while (!text.empty() && (text.back() == '\n' || text.back() == '\r'))
                text.pop_back();
            if (text == "quit")
                break;
            std::printf("echo:%s\n", text.c_str());
            std::fflush(stdout);
        }
        return 0;
    }
    if (mode == "exit") {
        return argc > 2 ? std::atoi(argv[2]) : 0;
    }
    if (mode == "sleep") {
        // Waits to be signalled, for the tests that end a session rather than
        // let it finish.
        for (;;) {
#ifdef _WIN32
            Sleep(1000);
#else
            pause();
#endif
        }
    }
    if (mode == "scenario") {
        emitScenario(argc > 2 ? argv[2] : "colour");
        return 0;
    }

    std::fprintf(stderr, "termstub: unknown mode %s\n", mode.c_str());
    return 2;
}
