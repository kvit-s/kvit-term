# kvit-term shell integration for zsh. See kvitterm.bash for what the marks
# are for; the mechanism differs because zsh has hooks where bash has strings.
#
# Source it from ~/.zshrc:
#     [ -f /path/to/kvitterm.zsh ] && . /path/to/kvitterm.zsh

if [ -n "${KVITTERM_SHELL_INTEGRATION:-}" ]; then
    return 0
fi
KVITTERM_SHELL_INTEGRATION=1

__kvitterm_osc() {
    printf '\033]%s\007' "$1"
}

# Before each prompt: the previous command's status, the directory, and the
# start of the prompt itself.
__kvitterm_precmd() {
    local status=$?
    __kvitterm_osc "133;D;${status}"
    __kvitterm_osc "7;file://${HOST}${PWD}"
    __kvitterm_osc "133;A"
}

# After a command line has been read and before it runs.
__kvitterm_preexec() {
    __kvitterm_osc "133;C"
}

autoload -Uz add-zsh-hook 2>/dev/null
if command -v add-zsh-hook >/dev/null 2>&1; then
    add-zsh-hook precmd __kvitterm_precmd
    add-zsh-hook preexec __kvitterm_preexec
else
    precmd_functions+=(__kvitterm_precmd)
    preexec_functions+=(__kvitterm_preexec)
fi

# %{ %} is zsh's way of saying these bytes occupy no columns.
if [[ "$PS1" != *"133;B"* ]]; then
    PS1="${PS1}%{$(printf '\033]133;B\007')%}"
fi
