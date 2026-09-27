#!/usr/bin/env bash
# Builds kvit-term-go: every package, and kvit-term-demo into build/.
#
#   ./build.sh               build
#   ./build.sh --test        also check formatting, run go vet and the tests
#   ./build.sh --race        the tests again under the race detector (needs cgo and gcc)
#   ./build.sh --cross       also build kvit-term-demo for windows/amd64, darwin/arm64,
#                            darwin/amd64 and linux/amd64 into build/<os>-<arch>/
#   ./build.sh --shots       draw sample screens into build/shots, the Qt library's
#                            docs/terminal.png stacked above the Go drawing of it
#   ./build.sh --bench       time the emulator on 16 MB of recorded output
#   ./build.sh --win         build kvit-term-demo for Windows onto D: and start it there
#   ./build.sh --win-test    run the pseudo-terminal, session and screen tests on Windows,
#                            through its pseudoconsole
#   ./build.sh --win-check   start kvit-term-demo on Windows on PowerShell, drive it through
#                            a scripted check, read it through UI Automation, and picture it
#   ./build.sh --run         start kvit-term-demo here (needs a display)
#
# Everything builds with cgo off, except --race. KVIT_WIN_DIR overrides where Windows
# builds go (default /mnt/d/projects/kvit-term-go).
set -euo pipefail
cd "$(dirname "$0")"
export CGO_ENABLED=0
test=0 race=0 cross=0 win=0 wintest=0 check=0 run=0 shots=0 bench=0
for a in "$@"; do
    case $a in
        --test) test=1 ;;
        --race) race=1 ;;
        --cross) cross=1 ;;
        --win) win=1 ;;
        --win-test) wintest=1 ;;
        --win-check) win=1 check=1 ;;
        --shots) shots=1 ;;
        --bench) bench=1 ;;
        --run) run=1 ;;
        -h|--help) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown option: $a" >&2; exit 2 ;;
    esac
done

mkdir -p build
go build ./...
go build -o build/kvit-term-demo ./cmd/kvit-term-demo

if [ $test = 1 ]; then
    unformatted=$(gofmt -l . | grep -v '^third_party/' || true)
    if [ -n "$unformatted" ]; then
        echo "not formatted with gofmt:" >&2
        echo "$unformatted" >&2
        exit 1
    fi
    go vet ./...
    GOOS=windows go vet ./...
    GOOS=darwin go vet ./...
    go test ./...
fi

if [ $race = 1 ]; then
    CGO_ENABLED=1 go test -race -count=1 ./...
fi

if [ $cross = 1 ]; then
    for target in windows/amd64 darwin/arm64 darwin/amd64 linux/amd64; do
        os=${target%/*} arch=${target#*/} ext=
        [ "$os" = windows ] && ext=.exe
        GOOS=$os GOARCH=$arch go build -o "build/$os-$arch/kvit-term-demo$ext" ./cmd/kvit-term-demo
    done
fi

if [ $shots = 1 ]; then
    rm -rf build/shots
    build/kvit-term-demo --shots build/shots
fi

if [ $bench = 1 ]; then
    go test ./screen -run '^$' -bench Feed -benchtime 5x
fi

dest=${KVIT_WIN_DIR:-/mnt/d/projects/kvit-term-go}

if [ $wintest = 1 ]; then
    # The tests built for Windows and run there, from WSL, with termstub
    # built beside them. The screen tests read their recordings from
    # screen/testdata, so they run in a copy of it.
    mkdir -p "$dest/test"
    GOOS=windows GOARCH=amd64 go build -o "$dest/test/termstub.exe" ./internal/termstub
    rm -rf "$dest/test/testdata" && cp -r screen/testdata "$dest/test/testdata"
    status=0
    for pkg in pty . screen; do
        name=$(basename "$(cd "$pkg" && pwd)")
        GOOS=windows GOARCH=amd64 go test -c -o "$dest/test/$name.test.exe" "./$pkg"
        echo "== $pkg on Windows"
        (cd "$dest/test" && KVIT_TERMSTUB="$(wslpath -w "$dest/test/termstub.exe")" WSLENV=KVIT_TERMSTUB \
            "./$name.test.exe" -test.timeout 180s -test.count 1 | tr -d '\r') || status=1
    done
    [ $status = 0 ] || exit 1
fi

if [ $win = 1 ]; then
    mkdir -p "$dest"
    GOOS=windows GOARCH=amd64 go build -o "$dest/kvit-term-demo.exe" ./cmd/kvit-term-demo
    if [ $check = 0 ]; then
        "$dest/kvit-term-demo.exe" &
        disown
    else
        cp tools/win-check.ps1 "$dest/"
        winDest=$(wslpath -w "$dest")
        # First as the program runs, with unison's OpenGL renderer: the
        # scripted check, what the screen-reader interface reports, and memory.
        "$dest/kvit-term-demo.exe" --check 25s &
        pid=$!
        sleep 14
        powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$winDest\\win-check.ps1" -Title "kvit-term check" | tr -d '\r'
        powershell.exe -NoProfile -Command '
            $p = Get-Process kvit-term-demo
            $c = (Get-Counter "\Process(kvit-term-demo)\Working Set - Private").CounterSamples[0].CookedValue
            "Windows memory: working set {0:N0} MB, private working set {1:N0} MB, private bytes {2:N0} MB" -f ($p.WorkingSet64 / 1MB), ($c / 1MB), ($p.PrivateMemorySize64 / 1MB)' | tr -d '\r'
        wait $pid
        # Then with unison's software renderer, for a picture of the window:
        # nothing reads back what the OpenGL renderer put on the screen.
        echo "== again with the software renderer, for a picture"
        UNISON_CPU_RENDERING=1 WSLENV=UNISON_CPU_RENDERING "$dest/kvit-term-demo.exe" --check 16s | grep -E 'first frame|check:|FAIL' &
        pid=$!
        sleep 12
        powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$winDest\\win-check.ps1" \
            -Title "kvit-term check" -Shot "$winDest\\check.png" | tr -d '\r' | grep saved
        wait $pid
        cp "$dest/check.png" build/win-check.png && echo "window picture: build/win-check.png"
    fi
fi

if [ $run = 1 ]; then
    build/kvit-term-demo
fi
