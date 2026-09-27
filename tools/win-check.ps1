# Reads what Windows' own screen-reader interface, UI Automation, reports
# about a running kvit-term-demo --check window, and saves a picture of that
# window. build.sh --win-check runs it while the check holds its window open.
#
#   powershell.exe -File win-check.ps1 -Title "kvit-term check" -Shot out.png
param([string]$Title = "kvit-term check", [string]$Shot = "")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes, System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class Win {
    [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, uint flags);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
    [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
    [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr h, int a, out RECT r, int size);
    public struct RECT { public int Left, Top, Right, Bottom; }
}
"@
$A = [System.Windows.Automation.AutomationElement]
$byName = New-Object System.Windows.Automation.PropertyCondition($A::NameProperty, $Title)
$win = $null
for ($i = 0; $i -lt 50 -and $win -eq $null; $i++) {
    $win = $A::RootElement.FindFirst([System.Windows.Automation.TreeScope]::Children, $byName)
    if ($win -eq $null) { Start-Sleep -Milliseconds 200 }
}
if ($win -eq $null) { "no window titled '$Title'"; exit 1 }
$hwnd = [IntPtr]$win.Current.NativeWindowHandle
"window: '$($win.Current.Name)', class $($win.Current.ClassName)"
# Keyboard focus is reported only in the window in front, so bring it there
# first, and say whether that worked.
[void][Win]::SetForegroundWindow($hwnd)
Start-Sleep -Milliseconds 400
"in front: $([Win]::GetForegroundWindow() -eq $hwnd)"

# The terminal: a read-only text control named "terminal".
$byTerm = New-Object System.Windows.Automation.PropertyCondition($A::NameProperty, "terminal")
$term = $win.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $byTerm)
if ($term -eq $null) { "no control named 'terminal'" } else {
    "terminal: control type $($term.Current.ControlType.ProgrammaticName), focus=$($term.Current.HasKeyboardFocus)"
    try {
        $vp = $term.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern)
        "  read-only: $($vp.Current.IsReadOnly)"
    } catch { "  no value pattern" }
    try {
        $tp = $term.GetCurrentPattern([System.Windows.Automation.TextPattern]::Pattern)
        $text = $tp.DocumentRange.GetText(-1)
        $lines = ($text -split "`n" | Where-Object { $_.Trim() -ne "" })
        "  text pattern reads $($lines.Count) non-empty lines; the last three:"
        foreach ($l in ($lines | Select-Object -Last 3)) { "    |$l" }
        $sel = $tp.GetSelection()
        if ($sel.Length -gt 0) {
            $before = $tp.DocumentRange.Clone()
            $before.MoveEndpointByRange([System.Windows.Automation.Text.TextPatternRangeEndpoint]::End, $sel[0],
                [System.Windows.Automation.Text.TextPatternRangeEndpoint]::Start)
            $b = $before.GetText(-1) -split "`n"
            "  caret on line $($b.Length) after: '$($b[-1])|'"
        }
    } catch { "  no text pattern: $_" }
}

if ($Shot -ne "") {
    # PrintWindow asks the window for its own pixels, so nothing in front of
    # it is captured.
    $r = New-Object Win+RECT
    [void][Win]::DwmGetWindowAttribute($hwnd, 9, [ref]$r, 16)
    $w = $r.Right - $r.Left; $h = $r.Bottom - $r.Top
    $bmp = New-Object System.Drawing.Bitmap $w, $h
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $full = New-Object Win+RECT
    [void][Win]::GetWindowRect($hwnd, [ref]$full)
    $all = New-Object System.Drawing.Bitmap ($full.Right - $full.Left), ($full.Bottom - $full.Top)
    $ga = [System.Drawing.Graphics]::FromImage($all)
    $hdc = $ga.GetHdc()
    [void][Win]::PrintWindow($hwnd, $hdc, 2)
    $ga.ReleaseHdc($hdc)
    $g.DrawImage($all, ($full.Left - $r.Left), ($full.Top - $r.Top))
    $bmp.Save($Shot, [System.Drawing.Imaging.ImageFormat]::Png)
    "saved $w x $h to $Shot"
}
