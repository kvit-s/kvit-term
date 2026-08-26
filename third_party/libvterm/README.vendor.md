# Vendored libvterm

Upstream: <https://www.leonerd.org.uk/code/libvterm/>, by Paul Evans.
Version: **0.3.3**, taken from the release tarball `libvterm-0.3.3.tar.gz`.
Licence: MIT, in `LICENSE` beside this file, unchanged.

## What was taken, and what was left

Vendored: `include/`, `src/`, `LICENSE`, `CONTRIBUTING`.

Left behind: `bin/` (upstream's three demonstration programs), `t/` (upstream's
test corpus and its Perl runner), `Makefile`, and `vterm.pc.in`. This repository
builds the library with its own CMake wrapper in `../CMakeLists.txt`.

## Why the release tarball rather than a git checkout

Upstream's `Makefile` generates the character-encoding tables
`src/encoding/*.inc` and `src/fullwidth.inc` from `.tbl` files using two Perl
scripts. The release tarball ships those three files already generated and
ships neither the `.tbl` inputs nor the scripts, so the rules never fire and
building needs no Perl on any platform. A copy taken from upstream's git
repository has the inputs instead of the outputs and would need Perl
everywhere, Windows included.

## Local changes

None. Any fix made here should be a separate, clearly-marked commit and should
be sent upstream.
