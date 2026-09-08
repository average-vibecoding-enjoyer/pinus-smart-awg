[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$patterns = [ordered]@{
    "VPN private key"     = '(?m)^\s*(?:PrivateKey|PresharedKey|HeaderProtectionKey)\s*=\s*[A-Za-z0-9+/]{40,}={0,2}\s*$'
    "Telegram bot token" = '\b[0-9]{8,12}:[A-Za-z0-9_-]{30,}\b'
    "Paylee API key"      = '\bmk_[A-Za-z0-9]{20,}\b'
    "Webhook secret"     = '\bwhsec_[A-Za-z0-9]{20,}\b'
    "PEM private key"     = '-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----'
}
$allowedTestFixtures = @{
    "core/conf/awg31_test.go" = @(
        "PrivateKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=",
        "HeaderProtectionKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="
    )
    "client/smart/awg31_test.go" = @(
        "HeaderProtectionKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="
    )
    "client/smart/settings_test.go" = @(
        "PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
    )
    "core/conf/parser_test.go" = @(
        "PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=",
        "PresharedKey = TrMvSoP4jYQlY6RIzBgbssQqY3vxI2Pi+y71lOWWXX0="
    )
}
$forbiddenExtensions = @(".conf", ".dpapi", ".pfx", ".p12", ".pem", ".key")
$findings = [System.Collections.Generic.List[string]]::new()

Push-Location $repositoryRoot
try {
    $trackedFiles = @(git ls-files --cached --others --exclude-standard)
    if ($LASTEXITCODE -ne 0) {
        throw "git ls-files failed"
    }

    foreach ($relativePath in $trackedFiles) {
        $extension = [IO.Path]::GetExtension($relativePath).ToLowerInvariant()
        if ($forbiddenExtensions -contains $extension) {
            $findings.Add("forbidden tracked file: $relativePath")
            continue
        }

        $fullPath = Join-Path $repositoryRoot $relativePath
        if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
            continue
        }
        if ((Get-Item -LiteralPath $fullPath).Length -gt 5MB) {
            continue
        }

        try {
            $content = [IO.File]::ReadAllText($fullPath)
        } catch {
            continue
        }
        if ($content.Contains([char]0)) {
            continue
        }

        foreach ($entry in $patterns.GetEnumerator()) {
            foreach ($match in [regex]::Matches($content, $entry.Value)) {
                $matchedText = $match.Value.Trim()
                $allowed = $allowedTestFixtures.ContainsKey($relativePath) -and
                    $allowedTestFixtures[$relativePath] -contains $matchedText
                if ($allowed) {
                    continue
                }
                $line = [regex]::Matches($content.Substring(0, $match.Index), "\r?\n").Count + 1
                $findings.Add("$relativePath`:$line [$($entry.Key)]")
            }
        }
    }
} finally {
    Pop-Location
}

if ($findings.Count -gt 0) {
    $findings | Sort-Object -Unique | ForEach-Object { Write-Error -Message $_ -ErrorAction Continue }
    throw "Secret scan found $($findings.Count) possible leak(s)."
}

Write-Host "Secret scan passed: no private profiles, credentials, or signing keys are tracked."
