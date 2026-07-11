param(
    [ValidateSet("amd64", "arm64", "386")]
    [string[]]$Architectures = @("amd64", "arm64", "386"),
    [switch]$SkipInstallers
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Client = Join-Path $Root "client"
$Core = Join-Path $Root "core"
$Cache = Join-Path $Root ".cache"
$Build = Join-Path $Root ".build"
$Release = Join-Path $Root "release"
$GoVersion = "1.26.5"
$GoURL = "https://go.dev/dl/go$GoVersion.windows-amd64.zip"
$GoArchiveSHA256 = "97E6B2A833B6D89F9FF17D25419AC0A7E3B482A044E9AB18CDEF834BD834FD38"
$EngineRepository = "https://github.com/hoaxisr/amnezia-box.git"
$EngineTag = "v1.13.13-awg2.1"
$EngineCommit = "f40548f91a14582975096d0310e3c6afd44656f8"
$EngineTags = "with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_awg"
$EnginePatch = Join-Path $Root "engine\amnezia-box-security.patch"
$EnginePatchSHA256 = "1743697E8871743EBAA1ACF9BA474A363F8881E4202B4581B34A3B8163358A95"
$EnginePatchedGoSumSHA256 = "4E8BC0FAB40415381BD1E679B7B6C6DD7E2284B13F3F9EE26712ADD8BF8E60A4"
$GovulncheckVersion = "v1.6.0"
$WintunURL = "https://www.wintun.net/builds/wintun-0.14.1.zip"
$WintunArchiveSHA256 = "07C256185D6EE3652E09FA55C0B673E2624B565E02C4B9091C79CA7D2F24EF51"
$WixURL = "https://github.com/wixtoolset/wix3/releases/download/wix3141rtm/wix314-binaries.zip"
$WixArchiveSHA256 = "6AC824E1642D6F7277D0ED7EA09411A508F6116BA6FAE0AA5F2C7DAA2FF43D31"

$ArchitectureInfo = @{
    amd64 = [pscustomobject]@{ Label = "x64"; Wintun = "amd64"; Wix = "x64"; Machine = 0x8664 }
    arm64 = [pscustomobject]@{ Label = "arm64"; Wintun = "arm64"; Wix = "arm64"; Machine = 0xAA64 }
    "386" = [pscustomobject]@{ Label = "x86"; Wintun = "x86"; Wix = "x86"; Machine = 0x014C }
}

function Get-NormalizedPath([string]$Path) {
    return [IO.Path]::GetFullPath($Path).TrimEnd([IO.Path]::DirectorySeparatorChar)
}

$RootPath = Get-NormalizedPath $Root
$RootPrefix = $RootPath + [IO.Path]::DirectorySeparatorChar

function Assert-PathUnderRoot([string]$Path) {
    $fullPath = Get-NormalizedPath $Path
    if (-not $fullPath.StartsWith($RootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside the repository: $fullPath"
    }
}

function Reset-Directory([string]$Path) {
    Assert-PathUnderRoot $Path
    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

function Get-SHA256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToUpperInvariant()
}

function Get-VerifiedDownload([string]$URL, [string]$Destination, [string]$ExpectedSHA256) {
    Assert-PathUnderRoot $Destination
    if (Test-Path -LiteralPath $Destination) {
        if ((Get-SHA256 $Destination) -eq $ExpectedSHA256) {
            return
        }
        Remove-Item -LiteralPath $Destination -Force
    }

    $temporary = "$Destination.download"
    if (Test-Path -LiteralPath $temporary) {
        Remove-Item -LiteralPath $temporary -Force
    }
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Destination) | Out-Null
    & curl.exe -L --fail --silent --show-error --output $temporary $URL
    if ($LASTEXITCODE -ne 0) {
        throw "Download failed: $URL"
    }
    $actual = Get-SHA256 $temporary
    if ($actual -ne $ExpectedSHA256) {
        Remove-Item -LiteralPath $temporary -Force
        throw "SHA-256 mismatch for $URL. Got $actual"
    }
    Move-Item -LiteralPath $temporary -Destination $Destination
}

function Assert-GoVersion([string]$Go) {
    $actual = (& $Go version).Trim()
    if ($LASTEXITCODE -ne 0 -or $actual -notmatch "^go version go$([regex]::Escape($GoVersion)) ") {
        throw "Pinus Smart AWG requires Go $GoVersion exactly. Got: $actual"
    }
}

function Resolve-Go {
    if ($env:PINUS_GO -and (Test-Path -LiteralPath $env:PINUS_GO)) {
        $go = (Resolve-Path -LiteralPath $env:PINUS_GO).Path
        Assert-GoVersion $go
        return $go
    }
    $command = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($command) {
        try {
            Assert-GoVersion $command.Source
            return $command.Source
        } catch {
            Write-Host "Ignoring non-pinned system Go: $($_.Exception.Message)"
        }
    }

    $archive = Join-Path $Cache "go$GoVersion.windows-amd64.zip"
    Get-VerifiedDownload $GoURL $archive $GoArchiveSHA256
    $toolchainRoot = Join-Path $Cache "go$GoVersion"
    $go = Join-Path $toolchainRoot "go\bin\go.exe"
    if (-not (Test-Path -LiteralPath $go)) {
        Reset-Directory $toolchainRoot
        Expand-Archive -LiteralPath $archive -DestinationPath $toolchainRoot
    }
    Assert-GoVersion $go
    return $go
}

function Invoke-Go([string]$Go, [string[]]$Arguments) {
    & $Go @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Go command failed: go $($Arguments -join ' ')"
    }
}

function Get-PeMachine([string]$Path) {
    $stream = [IO.File]::OpenRead($Path)
    try {
        $reader = New-Object IO.BinaryReader($stream)
        $stream.Position = 0x3C
        $peOffset = $reader.ReadInt32()
        $stream.Position = $peOffset
        if ($reader.ReadUInt32() -ne 0x00004550) {
            throw "Invalid PE signature: $Path"
        }
        return $reader.ReadUInt16()
    } finally {
        $stream.Dispose()
    }
}

function Assert-PeMachine([string]$Path, [int]$Expected) {
    $actual = Get-PeMachine $Path
    if ($actual -ne $Expected) {
        throw ("Unexpected PE architecture for {0}: 0x{1:X4}, expected 0x{2:X4}" -f $Path, $actual, $Expected)
    }
}

function Resolve-SignTool {
    $command = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }

    $programFilesX86 = ${env:ProgramFiles(x86)}
    if ($programFilesX86) {
        $kitsBin = Join-Path $programFilesX86 "Windows Kits\10\bin"
        if (Test-Path -LiteralPath $kitsBin) {
            $candidate = Get-ChildItem -LiteralPath $kitsBin -Filter signtool.exe -Recurse -File -ErrorAction SilentlyContinue |
                Where-Object FullName -Match '\\x64\\signtool\.exe$' |
                Sort-Object FullName -Descending |
                Select-Object -First 1
            if ($candidate) {
                return $candidate.FullName
            }
        }
    }
    return $null
}

function Sign-Artifact([string]$Path) {
    if (-not $env:PINUS_SIGN_CERT_THUMBPRINT) {
        return
    }
    $signTool = Resolve-SignTool
    if (-not $signTool) {
        throw "PINUS_SIGN_CERT_THUMBPRINT is set, but signtool.exe was not found."
    }
    & $signTool sign /sha1 $env:PINUS_SIGN_CERT_THUMBPRINT /fd SHA256 /td SHA256 /tr "http://timestamp.digicert.com" /d "Pinus Smart AWG" $Path
    if ($LASTEXITCODE -ne 0) {
        throw "Signing failed: $Path"
    }
}

$Go = Resolve-Go
$VersionSource = Get-Content -LiteralPath (Join-Path $Client "version\version.go") -Raw
if ($VersionSource -notmatch 'Number\s*=\s*"([0-9]+\.[0-9]+\.[0-9]+)"') {
    throw "Unable to determine the application version."
}
$Version = $Matches[1]

New-Item -ItemType Directory -Force -Path $Cache | Out-Null
Reset-Directory $Build
Reset-Directory $Release

$WintunArchive = Join-Path $Cache "wintun-0.14.1.zip"
Get-VerifiedDownload $WintunURL $WintunArchive $WintunArchiveSHA256
$WintunRoot = Join-Path $Cache "wintun"
$WintunProbe = Join-Path $WintunRoot "wintun\bin\amd64\wintun.dll"
if (-not (Test-Path -LiteralPath $WintunProbe)) {
    Reset-Directory $WintunRoot
    Expand-Archive -LiteralPath $WintunArchive -DestinationPath $WintunRoot
}

$EngineSource = Join-Path $Cache "amnezia-box"
$engineReady = Test-Path -LiteralPath (Join-Path $EngineSource ".git")
if ($engineReady) {
    $currentCommit = (& git -C $EngineSource rev-parse HEAD).Trim()
    $workingTree = (& git -C $EngineSource status --porcelain | Out-String).Trim()
    $engineReady = $LASTEXITCODE -eq 0 -and $currentCommit -eq $EngineCommit -and $workingTree.Length -eq 0
}
if (-not $engineReady) {
    if (Test-Path -LiteralPath $EngineSource) {
        Assert-PathUnderRoot $EngineSource
        Remove-Item -LiteralPath $EngineSource -Recurse -Force
    }
    & git clone --depth 1 --branch $EngineTag $EngineRepository $EngineSource
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to clone the pinned amnezia-box source."
    }
}
$currentCommit = (& git -C $EngineSource rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $currentCommit -ne $EngineCommit) {
    throw "amnezia-box commit mismatch: $currentCommit"
}
if ((Get-SHA256 $EnginePatch) -ne $EnginePatchSHA256) {
    throw "amnezia-box security patch SHA-256 mismatch."
}
& git -C $EngineSource apply --check $EnginePatch
if ($LASTEXITCODE -ne 0) {
    throw "amnezia-box security patch no longer applies cleanly."
}
& git -C $EngineSource apply $EnginePatch
if ($LASTEXITCODE -ne 0) {
    throw "Unable to apply the amnezia-box security patch."
}
Push-Location $EngineSource
try {
    Invoke-Go $Go @("mod", "tidy")
} finally {
    Pop-Location
}
if ((Get-SHA256 (Join-Path $EngineSource "go.sum")) -ne $EnginePatchedGoSumSHA256) {
    throw "Patched amnezia-box go.sum is not reproducible."
}

if (-not $SkipInstallers) {
    $WixArchive = Join-Path $Cache "wix314-binaries.zip"
    Get-VerifiedDownload $WixURL $WixArchive $WixArchiveSHA256
    $WixRoot = Join-Path $Cache "wix314"
    if (-not (Test-Path -LiteralPath (Join-Path $WixRoot "candle.exe"))) {
        Reset-Directory $WixRoot
        Expand-Archive -LiteralPath $WixArchive -DestinationPath $WixRoot
    }
}

$oldGOOS = $env:GOOS
$oldGOARCH = $env:GOARCH
$oldCGO = $env:CGO_ENABLED
$oldToolchain = $env:GOTOOLCHAIN
try {
    $env:GOTOOLCHAIN = "local"
    $env:CGO_ENABLED = "0"

    Push-Location $Client
    try {
        Invoke-Go $Go @("test", "-count=1", "./...")
        Invoke-Go $Go @("run", "golang.org/x/vuln/cmd/govulncheck@$GovulncheckVersion", "./...")
        Get-ChildItem -Filter "resource_windows_*.syso" -ErrorAction SilentlyContinue | Remove-Item -Force
        Invoke-Go $Go @("run", "github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0", "-platform-specific", "versioninfo.json")
    } finally {
        Pop-Location
    }

    Push-Location $Core
    try {
        Invoke-Go $Go @("test", "-count=1", "./...")
        Invoke-Go $Go @("run", "golang.org/x/vuln/cmd/govulncheck@$GovulncheckVersion", "./...")
    } finally {
        Pop-Location
    }

    $EngineArchive = Join-Path $Release "amnezia-box-source-$EngineCommit.zip"
    & git -C $EngineSource archive --format=zip "--output=$EngineArchive" HEAD
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to create the corresponding amnezia-box source archive."
    }
    $EnginePatchAsset = Join-Path $Release "amnezia-box-security-$EnginePatchSHA256.patch"
    Copy-Item -LiteralPath $EnginePatch -Destination $EnginePatchAsset
    Push-Location $EngineSource
    try {
        Invoke-Go $Go @("test", "-count=1", "-tags", $EngineTags, ".\cmd\sing-box")
        Invoke-Go $Go @("run", "golang.org/x/vuln/cmd/govulncheck@$GovulncheckVersion", "-tags", $EngineTags, ".\cmd\sing-box")
    } finally {
        Pop-Location
    }

    $manifestArchitectures = @()
    foreach ($architecture in $Architectures) {
        $info = $ArchitectureInfo[$architecture]
        $dependencyDirectory = Join-Path $Build "dependencies\$architecture"
        $bundleDirectory = Join-Path $Build "bundles\$architecture"
        New-Item -ItemType Directory -Force -Path $dependencyDirectory, $bundleDirectory | Out-Null

        $engine = Join-Path $dependencyDirectory "amnezia-box.exe"
        $env:GOOS = "windows"
        $env:GOARCH = $architecture
        Push-Location $EngineSource
        try {
            Invoke-Go $Go @(
                "build",
                "-trimpath",
                "-tags", $EngineTags,
                "-ldflags", "-s -w",
                "-o", $engine,
                ".\cmd\sing-box"
            )
        } finally {
            Pop-Location
        }
        Assert-PeMachine $engine $info.Machine
        $engineMetadata = (& $Go version -m $engine | Out-String)
        if ($engineMetadata -notmatch [regex]::Escape($EngineCommit)) {
            throw "The $architecture engine does not contain the pinned revision."
        }
        Sign-Artifact $engine
        $engineHash = Get-SHA256 $engine

        $wintun = Join-Path $WintunRoot "wintun\bin\$($info.Wintun)\wintun.dll"
        if (-not (Test-Path -LiteralPath $wintun)) {
            throw "Wintun DLL not found for $architecture."
        }
        Assert-PeMachine $wintun $info.Machine
        $wintunHash = Get-SHA256 $wintun

        $clientExe = Join-Path $bundleDirectory "PinusSmartAWG.exe"
        $ldflags = "-H=windowsgui -s -w -X github.com/amnezia-vpn/amneziawg-windows-client/smart.EngineSHA256=$engineHash -X github.com/amnezia-vpn/amneziawg-windows-client/manager.wintunDLLSHA256=$wintunHash"
        Push-Location $Client
        try {
            Invoke-Go $Go @("build", "-trimpath", "-buildvcs=false", "-ldflags", $ldflags, "-o", $clientExe, ".")
        } finally {
            Pop-Location
        }
        Assert-PeMachine $clientExe $info.Machine
        Sign-Artifact $clientExe

        Copy-Item -LiteralPath $engine -Destination (Join-Path $bundleDirectory "amnezia-box.exe")
        Copy-Item -LiteralPath $wintun -Destination (Join-Path $bundleDirectory "wintun.dll")
        Copy-Item -LiteralPath (Join-Path $Root "packaging\README.txt") -Destination (Join-Path $bundleDirectory "README.txt")
        Copy-Item -LiteralPath (Join-Path $Client "COPYING") -Destination (Join-Path $bundleDirectory "LICENSE.txt")
        Copy-Item -LiteralPath (Join-Path $EngineSource "LICENSE") -Destination (Join-Path $bundleDirectory "ENGINE-LICENSE.txt")
        Copy-Item -LiteralPath (Join-Path $WintunRoot "wintun\LICENSE.txt") -Destination (Join-Path $bundleDirectory "WINTUN-LICENSE.txt")
        Copy-Item -LiteralPath (Join-Path $Root "packaging\THIRD-PARTY-NOTICES.txt") -Destination (Join-Path $bundleDirectory "THIRD-PARTY-NOTICES.txt")

        $portableArchive = Join-Path $Release "Pinus-Smart-AWG-$Version-windows-$($info.Label)-portable.zip"
        Compress-Archive -Path (Join-Path $bundleDirectory "*") -DestinationPath $portableArchive -CompressionLevel Optimal

        if (-not $SkipInstallers) {
            $wixObjectDirectory = Join-Path $Build "wix\$architecture"
            New-Item -ItemType Directory -Force -Path $wixObjectDirectory | Out-Null
            $wixObject = Join-Path $wixObjectDirectory "pinus-smart-awg.wixobj"
            $msi = Join-Path $Release "Pinus-Smart-AWG-$Version-windows-$($info.Label).msi"
            $candle = Join-Path $WixRoot "candle.exe"
            $light = Join-Path $WixRoot "light.exe"
            & $candle -nologo -arch $info.Wix "-dAppVersion=$Version" "-dPlatform=$($info.Wix)" "-dArchLabel=$($info.Label)" "-dBundleDir=$bundleDirectory" "-dIconPath=$(Join-Path $Client 'ui\icon\pinus-smart-awg.ico')" "-dLicenseRtf=$(Join-Path $Root 'packaging\LICENSE.rtf')" -out $wixObject (Join-Path $Root "installer\pinus-smart-awg.wxs")
            if ($LASTEXITCODE -ne 0) {
                throw "WiX candle failed for $architecture."
            }
            & $light -nologo -spdb -ext (Join-Path $WixRoot "WixUIExtension.dll") -cultures:ru-ru -out $msi $wixObject
            if ($LASTEXITCODE -ne 0) {
                throw "WiX light failed for $architecture."
            }
            Sign-Artifact $msi
        }

        $manifestArchitectures += [pscustomobject]@{
            go_arch = $architecture
            release_label = $info.Label
            client_sha256 = Get-SHA256 $clientExe
            engine_sha256 = $engineHash
            wintun_sha256 = $wintunHash
        }
    }

    $goVersion = (& $Go version).Trim()
    $manifest = [ordered]@{
        product = "Pinus Smart AWG"
        version = $Version
        generated_utc = [DateTime]::UtcNow.ToString("o")
        go = $goVersion
        signed = [bool]$env:PINUS_SIGN_CERT_THUMBPRINT
        amnezia_box = [ordered]@{
            repository = $EngineRepository
            tag = $EngineTag
            commit = $EngineCommit
            build_tags = $EngineTags
            security_patch = [IO.Path]::GetFileName($EnginePatchAsset)
            security_patch_sha256 = $EnginePatchSHA256
            patched_go_sum_sha256 = $EnginePatchedGoSumSHA256
        }
        wintun = [ordered]@{
            version = "0.14.1"
            archive_url = $WintunURL
            archive_sha256 = $WintunArchiveSHA256
        }
        architectures = $manifestArchitectures
    }
    $manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Release "release-manifest.json") -Encoding UTF8

    $hashLines = Get-ChildItem -LiteralPath $Release -File |
        Where-Object Name -ne "SHA256SUMS.txt" |
        Sort-Object Name |
        ForEach-Object { "$(Get-SHA256 $_.FullName) *$($_.Name)" }
    [IO.File]::WriteAllLines((Join-Path $Release "SHA256SUMS.txt"), $hashLines, (New-Object Text.UTF8Encoding($false)))
} finally {
    $env:GOOS = $oldGOOS
    $env:GOARCH = $oldGOARCH
    $env:CGO_ENABLED = $oldCGO
    $env:GOTOOLCHAIN = $oldToolchain
}

Write-Host "Release assets: $Release"
Get-ChildItem -LiteralPath $Release -File | Sort-Object Name | Select-Object Name, Length | Format-Table -AutoSize
