<#
.SYNOPSIS
    One-liner installer for gh-aw-fleet on Windows.

.DESCRIPTION
    Downloads the matching gh-aw-fleet release archive for the current
    architecture (amd64 / arm64), verifies its SHA-256 checksum against the
    release's checksums.txt, and extracts the binary into the install
    directory. Optionally adds the install directory to the user-scope PATH.

.PARAMETER Version
    Release tag to install (e.g. "v0.2.0"). Defaults to the latest release.

.PARAMETER InstallDir
    Directory to install gh-aw-fleet.exe into. Defaults to
    $env:LOCALAPPDATA\gh-aw-fleet\bin.

.PARAMETER NoPath
    Skip the user-scope PATH update.

.PARAMETER BaseUrl
    Internal-only override for the release asset base URL. Used by CI for
    the checksum-tamper test. Not part of the public interface.

.EXAMPLE
    iwr -UseBasicParsing https://raw.githubusercontent.com/rshade/gh-aw-fleet/main/install.ps1 | iex

.EXAMPLE
    $installer = [ScriptBlock]::Create((iwr -UseBasicParsing https://raw.githubusercontent.com/rshade/gh-aw-fleet/main/install.ps1).Content)
    & $installer -Version v0.2.0 -InstallDir "$env:LOCALAPPDATA\gh-aw-fleet\bin" -NoPath
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'gh-aw-fleet\bin'),
    [switch]$NoPath,
    [Parameter(DontShow)]
    [string]$BaseUrl
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Repo    = 'rshade/gh-aw-fleet'
$Project = 'gh-aw-fleet'
$DefaultBaseUrl = "https://github.com/$Repo/releases/download"

function Get-Arch {
    $a = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($a) {
        'X64'   { return 'amd64' }
        'Arm64' { return 'arm64' }
        default { throw "unsupported architecture: $a" }
    }
}

function Resolve-ReleaseVersion {
    param([string]$Requested)
    if ($Requested) { return $Requested }
    $api = "https://api.github.com/repos/$Repo/releases/latest"
    $headers = @{}
    $token = if ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } elseif ($env:GH_TOKEN) { $env:GH_TOKEN } else { $null }
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    $resp = Invoke-RestMethod -Uri $api -Headers $headers -UseBasicParsing
    if (-not $resp.tag_name) {
        throw "could not parse tag_name from $api"
    }
    return $resp.tag_name
}

function Get-Archive {
    param(
        [Parameter(Mandatory)] [string]$Ver,
        [Parameter(Mandatory)] [string]$Arch,
        [Parameter(Mandatory)] [string]$TmpDir,
        [string]$BaseUrl
    )
    $verStrip  = $Ver.TrimStart('v')
    $archive   = "${Project}_${verStrip}_windows_${Arch}.zip"
    $checksums = "${Project}_${verStrip}_checksums.txt"
    $base      = "$( if ($BaseUrl) { $BaseUrl } else { $DefaultBaseUrl } )/$Ver"

    $archivePath   = Join-Path $TmpDir $archive
    $checksumsPath = Join-Path $TmpDir $checksums

    Write-Information "Downloading $archive" -InformationAction Continue
    Invoke-WebRequest -Uri "$base/$archive"   -OutFile $archivePath   -UseBasicParsing
    Write-Information "Downloading $checksums" -InformationAction Continue
    Invoke-WebRequest -Uri "$base/$checksums" -OutFile $checksumsPath -UseBasicParsing

    Write-Information 'Verifying SHA-256' -InformationAction Continue
    $line = Select-String -Path $checksumsPath -Pattern ([regex]::Escape($archive)) | Select-Object -First 1
    if (-not $line) {
        throw "no checksum entry for $archive in $checksums"
    }
    $expected = ($line.Line -split '\s+')[0].ToLowerInvariant()
    $actual   = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expected -ne $actual) {
        throw "checksum verification failed for ${archive}: expected $expected, got $actual"
    }

    return $archivePath
}

function Install-Binary {
    param(
        [Parameter(Mandatory)] [string]$ArchivePath,
        [Parameter(Mandatory)] [string]$Dest
    )
    New-Item -ItemType Directory -Path $Dest -Force | Out-Null
    Expand-Archive -Path $ArchivePath -DestinationPath $Dest -Force
    Write-Information "Installed $(Join-Path $Dest "$Project.exe")" -InformationAction Continue
}

function Update-UserPath {
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Mandatory)] [string]$Dest,
        [switch]$SkipPath
    )
    if ($SkipPath) { return }

    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrEmpty($current)) { $current = '' }

    $entries = $current.Split(';', [StringSplitOptions]::RemoveEmptyEntries)
    foreach ($e in $entries) {
        if ($e.TrimEnd('\') -ieq $Dest.TrimEnd('\')) {
            return
        }
    }

    $newPath = if ($current) { "$current;$Dest" } else { $Dest }
    if ($PSCmdlet.ShouldProcess($Dest, 'Add to user PATH')) {
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Information '' -InformationAction Continue
        Write-Information "Added $Dest to user PATH." -InformationAction Continue
        Write-Information 'Open a new shell to pick it up (current session uses the old PATH).' -InformationAction Continue
    }
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    $arch = Get-Arch
    $ver  = Resolve-ReleaseVersion -Requested $Version
    Write-Information "Installing $Project $ver (windows_$arch) into $InstallDir" -InformationAction Continue
    $archivePath = Get-Archive -Ver $ver -Arch $arch -TmpDir $tmp -BaseUrl $BaseUrl
    Install-Binary -ArchivePath $archivePath -Dest $InstallDir
    Update-UserPath -Dest $InstallDir -SkipPath:$NoPath
    Write-Information '' -InformationAction Continue
    Write-Information "Run '$Project --help' to get started." -InformationAction Continue
}
finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
