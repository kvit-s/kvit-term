# vtdiff: the emulator's regression test, and how its references were made

`screen/vtdiff_test.go` feeds every stream in `screen/testdata/vtdiff/` to
the Go screen and compares the result, cell by cell, with what libvterm
0.3.3 made of the same bytes. Those libvterm screens are stored in
`screen/testdata/vtdiff/libvterm/`, one file per stream, so the test does not
need libvterm.

The streams are recordings of programs in a pseudo-terminal; the tools here
record new streams and print reference screens.

## The streams

- `streams/`: 19 recordings of programs in an 80 × 24 pseudo-terminal, made
  with `rec.sh` (`ls`, `git log`, `cat`, `less`, `nano`, `top`, `tmux`,
  `whiptail`, and vim plain and with three colour schemes; a `.cut` copy stops
  before the program leaves the alternate screen), plus `synth-sections` and
  five streams of 3,000 random operations from `synth.py`, and `smoke`.
- `micro/`: 47 streams of one feature each, made by `micro.py`, so a
  difference there names its cause.

## Recording the libvterm screens again

`vtdump.c` prints a screen in the format the test reads: the cursor, each
row's text with `¤` for the right half of a wide character, and the style of
every cell not in the default one. It builds against libvterm 0.3.3's source, fetched separately:

```sh
curl -L https://github.com/neovim/libvterm/archive/v0.3.3.tar.gz | tar -xz -C /tmp
gcc -O2 -std=c99 -D_DEFAULT_SOURCE -I/tmp/third_party/libvterm/include \
    -I/tmp/third_party/libvterm/src -o /tmp/vtdump tools/vtdiff/vtdump.c \
    /tmp/third_party/libvterm/src/*.c
for f in screen/testdata/vtdiff/{streams,micro}/*.bin; do
    n=$(basename "$(dirname "$f")")-$(basename "$f" .bin)
    /tmp/vtdump 24 80 "$f" > "screen/testdata/vtdiff/libvterm/$n.txt"
done
```

The bundled reference screens were made with libvterm 0.3.3.
