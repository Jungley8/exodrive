#!/usr/bin/env pwsh
# Install Exodrive on Windows. Read this file before you run it.
#
#   irm https://jungley8.github.io/exodrive/install.ps1 | iex
#
# Options, when you save the file and run it:
#   -Version v0.8.0   Install one version. The default is the newest release.
#   -Dir C:\Tools     Install to this folder. The default is
#                     %LOCALAPPDATA%\Programs\Exodrive.
#
# The script downloads the archive for this machine and checks it against
# SHA256SUMS. A mismatch stops the install before it writes anything.
param(
    [string]$Version = "",
    [string]$Dir = ""
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$repo = "Jungley8/exodrive"

function Get-LatestTag {
    $url = "https://github.com/$repo/releases/latest"
    $response = Invoke-WebRequest -Uri $url -MaximumRedirection 5 -UseBasicParsing
    $final = $response.BaseResponse.ResponseUri
    if (-not $final) { $final = $response.BaseResponse.RequestMessage.RequestUri }
    if (-not $final) { throw "Cannot read the newest release from $url." }
    return ($final.AbsoluteUri -split "/")[-1]
}

function Get-ExpectedHash {
    param([string]$Path, [string]$Asset)
    foreach ($line in Get-Content -Path $Path) {
        $fields = $line -split "\s+"
        if ($fields.Count -lt 2) { continue }
        $name = $fields[1] -replace "^\./", ""
        if ($name -eq $Asset) { return $fields[0] }
    }
    throw "SHA256SUMS lists no $Asset. Nothing was installed."
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default {
        throw "Exodrive has no package for $env:PROCESSOR_ARCHITECTURE. Download it from https://github.com/$repo/releases."
    }
}

$tag = $Version
if (-not $tag) { $tag = Get-LatestTag }
if (-not $tag.StartsWith("v")) { $tag = "v$tag" }

if (-not $Dir) {
    $base = $env:LOCALAPPDATA
    if (-not $base) { $base = $HOME }
    $Dir = Join-Path $base "Programs\Exodrive"
}

$asset = "exodrive_${tag}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$tag"
$work = Join-Path ([System.IO.Path]::GetTempPath()) ("exodrive-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $work -Force | Out-Null

try {
    $zip = Join-Path $work $asset
    Invoke-WebRequest -Uri "$base/$asset" -OutFile $zip -UseBasicParsing

    $sums = Join-Path $work "SHA256SUMS"
    Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile $sums -UseBasicParsing

    $want = Get-ExpectedHash -Path $sums -Asset $asset
    $got = (Get-FileHash -Path $zip -Algorithm SHA256).Hash
    if ($got -ne $want.ToUpper()) {
        throw "The download does not match SHA256SUMS. Nothing was installed."
    }

    Expand-Archive -Path $zip -DestinationPath $work -Force
    $inner = Join-Path $work "exodrive_windows_$arch"
    if (-not (Test-Path (Join-Path $inner "exodrive.exe"))) {
        throw "The archive holds no exodrive.exe. Nothing was installed."
    }

    New-Item -ItemType Directory -Path $Dir -Force | Out-Null
    Copy-Item -Path (Join-Path $inner "*") -Destination $Dir -Recurse -Force
}
finally {
    Remove-Item -Path $work -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "Exodrive $tag is installed in $Dir."

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $Dir) {
    Write-Host "$Dir is not on PATH. Add it to run exodrive in a terminal:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', `"$userPath;$Dir`", 'User')"
}

Write-Host "Start the window: $Dir\exodrive-gui.exe"
