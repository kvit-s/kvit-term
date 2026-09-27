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
        # The directory as a file URL: the file system's own path rather than
        # PowerShell's provider path, with forward slashes after a slash of its
        # own (file://host/C:/Users/x, or file://host///server/share for a
        # network path), and the characters a URL cannot hold escaped.
        $path = $PWD.ProviderPath -replace '\\', '/'
        $path = $path -replace '%', '%25' -replace ' ', '%20' -replace '#', '%23' -replace '\?', '%3F'
        __KvitTermOsc ("7;file://{0}/{1}" -f [System.Net.Dns]::GetHostName(), $path)
        __KvitTermOsc "133;A"
        $text = __KvitTermOriginalPrompt
        __KvitTermOsc "133;B"
        return $text
    }
}

# The start of a command's output. PowerShell has no hook between reading a
# line and running it, but PSReadLine, which reads the line, is a function
# that can be wrapped, as Visual Studio Code's own snippet does: once the
# line has been read and its line break written, report that the command is
# starting (133;C) and then its command line (633;E, with backslashes,
# semicolons and control characters escaped). Without PSReadLine there is
# no C mark, and commands are not recorded.
if ((Test-Path Function:\PSConsoleHostReadLine) -and -not (Test-Path Variable:Global:__KvitTermReadLine)) {
    $Global:__KvitTermReadLine = $function:PSConsoleHostReadLine
    function Global:PSConsoleHostReadLine {
        $line = $Global:__KvitTermReadLine.Invoke()
        $escape = [char] 27
        $bell = [char] 7
        $escaped = "$line" -replace '\\', '\\' -replace ';', '\x3b' -replace "`n", '\x0a' -replace "`r", '\x0d'
        [Console]::Write("$escape]133;C$bell$escape]633;E;$escaped$bell")
        $line
    }
}
