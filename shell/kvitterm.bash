# kvit-term shell integration for bash.
#
# Marks the boundaries of every prompt and command with the standard OSC 133
# sequences, and reports the working directory with OSC 7. From those an
# application knows where each command began and ended, what it exited with,
# and where the shell now is — which is what makes "re-run that command",
# "jump to the previous command", a pass or fail mark beside each one, and
# "open the next terminal where this one is" possible at all.
#
# Source it from ~/.bashrc:
#     [ -f /path/to/kvitterm.bash ] && . /path/to/kvitterm.bash
#
# Sourcing it in a terminal that does not understand the marks is harmless:
# they are escape sequences an unrecognising terminal ignores.

if [ -n "${KVITTERM_SHELL_INTEGRATION:-}" ]; then
    return 0
fi
KVITTERM_SHELL_INTEGRATION=1

__kvitterm_osc() {
    printf '\033]%s\007' "$1"
}

# Runs after each command, before the next prompt: report the exit status,
# then where we are, then that a prompt is starting.
__kvitterm_prompt_command() {
    local status=$?
    __kvitterm_osc "133;D;${status}"
    __kvitterm_osc "7;file://${HOSTNAME:-localhost}${PWD}"
    __kvitterm_osc "133;A"
    # Only meaningful on the bash 3.2 path below, where it says the next
    # command to run is one the user typed at this prompt.
    __kvitterm_at_prompt=1
    return $status
}

case "${PROMPT_COMMAND:-}" in
    *__kvitterm_prompt_command*) ;;
    "") PROMPT_COMMAND="__kvitterm_prompt_command" ;;
    *) PROMPT_COMMAND="__kvitterm_prompt_command; ${PROMPT_COMMAND}" ;;
esac

# The end of the prompt, so that what follows it is the command as typed. The
# \[ \] tell bash these bytes take no space on screen, without which it
# miscounts the line and redraws it wrongly.
case "${PS1}" in
    *133\;B*) ;;
    *) PS1="${PS1}\[$(printf '\033]133;B\007')\]" ;;
esac

# The start of a command's output, emitted after the line has been read and
# before it runs.
#
# PS0 is the natural place for it and exists only in bash 4.4 and later, which
# leaves out the bash 3.2 that macOS still ships. There, a DEBUG trap fires
# before every simple command instead, and a flag set when the prompt is drawn
# keeps it to one mark per command line rather than one per command in it.
__kvitterm_supports_ps0() {
    [ -n "${BASH_VERSINFO:-}" ] || return 1
    [ "${BASH_VERSINFO[0]}" -gt 4 ] && return 0
    [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -ge 4 ]
}

if __kvitterm_supports_ps0; then
    PS0="${PS0:-}$(printf '\033]133;C\007')"
else
    __kvitterm_at_prompt=0

    __kvitterm_debug_trap() {
        # Completion runs commands too, and so does the prompt command
        # itself; neither is a command the user typed.
        [ -n "${COMP_LINE:-}" ] && return
        case "$BASH_COMMAND" in
            __kvitterm_*) return ;;
        esac
        if [ "$__kvitterm_at_prompt" = 1 ]; then
            __kvitterm_at_prompt=0
            __kvitterm_osc "133;C"
        fi
    }

    trap '__kvitterm_debug_trap' DEBUG
fi
