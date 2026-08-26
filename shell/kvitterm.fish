# kvit-term shell integration for fish. See kvitterm.bash for what the marks
# are for.
#
# Source it from ~/.config/fish/config.fish:
#     test -f /path/to/kvitterm.fish; and source /path/to/kvitterm.fish

if set -q KVITTERM_SHELL_INTEGRATION
    exit 0
end
set -g KVITTERM_SHELL_INTEGRATION 1

function __kvitterm_osc
    printf '\033]%s\007' $argv[1]
end

function __kvitterm_prompt_start --on-event fish_prompt
    __kvitterm_osc "133;A"
end

function __kvitterm_preexec --on-event fish_preexec
    __kvitterm_osc "133;C"
end

function __kvitterm_postexec --on-event fish_postexec
    __kvitterm_osc "133;D;$status"
end

function __kvitterm_pwd --on-variable PWD
    __kvitterm_osc "7;file://"(hostname)"$PWD"
end

# fish redraws the prompt itself, so the end-of-prompt mark is appended by
# wrapping the user's fish_prompt rather than by editing a string.
if not functions -q __kvitterm_original_fish_prompt
    functions -c fish_prompt __kvitterm_original_fish_prompt
    function fish_prompt
        __kvitterm_original_fish_prompt
        __kvitterm_osc "133;B"
    end
end
