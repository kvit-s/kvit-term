# kvit-term shell integration for PowerShell. See kvitterm.bash for what the
# marks are for.
#
# Dot-source it from $PROFILE:
#     . /path/to/kvitterm.ps1

if ($env:KVITTERM_SHELL_INTEGRATION) { return }
$env:KVITTERM_SHELL_INTEGRATION = 1

function Global:__KvitTermOsc([string] $body) {
    $escape = [char] 27
    $bell = [char] 7
    Write-Host -NoNewline "$escape]$body$bell"
}

# PowerShell has one hook for all of this: the prompt function runs after each
# command and before the next prompt, so the previous command's status, the
# directory and the prompt marks are all emitted from here.
if (-not (Test-Path Function:Global:__KvitTermOriginalPrompt)) {
    Copy-Item Function:prompt Function:Global:__KvitTermOriginalPrompt

    function Global:prompt {
        $lastOk = $?
        $status = if ($lastOk) { 0 } else { 1 }
        if ($null -ne $global:LASTEXITCODE) { $status = $global:LASTEXITCODE }

        __KvitTermOsc "133;D;$status"
        __KvitTermOsc ("7;file://{0}{1}" -f [System.Net.Dns]::GetHostName(), $PWD.Path)
        __KvitTermOsc "133;A"
        $text = __KvitTermOriginalPrompt
        __KvitTermOsc "133;B"
        return $text
    }
}
