$ErrorActionPreference = "Stop"
$verifier = Join-Path $PSScriptRoot "verify_task4_manual_evidence.ps1"
$testRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("lapanggo-evidence-verifier-" + [guid]::NewGuid().ToString("N"))
$evidence = Join-Path $testRoot "evidence"
$document = Join-Path $testRoot "manifest.md"

New-Item -ItemType Directory -Path $evidence -Force | Out-Null
try {
    $artifact = Join-Path $evidence "artifact.log"
    Set-Content -LiteralPath $artifact -Value "deterministic evidence" -NoNewline
    $hash = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash
    Set-Content -LiteralPath $document -Value "SHA-256: ``artifact.log=$hash``"

    $passOutput = & $verifier -DocumentPaths @($document) -EvidenceDirectories @($evidence)
    if (@($passOutput) -notcontains "manual_evidence_verified=1") {
        throw "positive evidence verifier regression failed"
    }

    Add-Content -LiteralPath $artifact -Value "x" -NoNewline
    $failedClosed = $false
    try {
        & $verifier -DocumentPaths @($document) -EvidenceDirectories @($evidence) | Out-Null
    } catch {
        if ($_.Exception.Message -like "evidence_digest_mismatch:*") {
            $failedClosed = $true
        } else {
            throw
        }
    }
    if (-not $failedClosed) {
        throw "one-byte mutation did not fail evidence verification"
    }
    Write-Output "manual_evidence_verifier_regression=PASS"
} finally {
    Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}
