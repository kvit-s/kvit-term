#!/usr/bin/env bash
# rec NAME COMMAND: run COMMAND in an 80x24 pseudo-terminal, keep the bytes it wrote
name=$1; shift
TERM=xterm-256color LANG=C.UTF-8 GIT_PAGER=cat PAGER=cat timeout 20 script -q -E always -O $name.raw -c "stty rows 24 cols 80; $*" > /dev/null 2>&1 < /dev/null
python3 "$(dirname "$0")/strip.py" $name.raw $name.bin && rm -f $name.raw
printf '%-8s %7d bytes\n' $name $(stat -c %s $name.bin)
