$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$required = @(
    "docs/version_1_7_platform_finance_readiness.md",
    "docs/platform_finance_metric_dictionary.md",
    "docs/platform_finance_incident_runbook.md",
    "docs/mvp_known_limitations.md"
)

foreach ($relative in $required) {
    $path = Join-Path $repoRoot $relative
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "release_document_missing:$relative"
    }
    & git -C $repoRoot check-ignore --quiet -- $relative
    if ($LASTEXITCODE -eq 0) {
        throw "release_document_ignored:$relative"
    }
}

$known = Get-Content -LiteralPath (Join-Path $repoRoot "docs/mvp_known_limitations.md") -Raw
foreach ($requiredText in @(
    "v1.7",
    "MODE SIMULASI",
    "Payment gateway",
    "actual commission",
    "owner payable",
    "payout",
    "migration sampai ``024``"
)) {
    if (-not $known.Contains($requiredText)) {
        throw "known_limitations_contract_missing:$requiredText"
    }
}
foreach ($obsoleteText in @("version_1.2", "Superadmin dashboard belum tersedia", "migration sampai ``015``")) {
    if ($known.Contains($obsoleteText)) {
        throw "known_limitations_obsolete_claim:$obsoleteText"
    }
}

foreach ($relative in $required) {
    $path = Join-Path $repoRoot $relative
    $content = Get-Content -LiteralPath $path -Raw
    foreach ($match in [regex]::Matches($content, '\[[^\]]+\]\((?!https?://|#)([^)]+\.md)(?:#[^)]+)?\)')) {
        $target = Join-Path (Split-Path -Parent $path) $match.Groups[1].Value
        if (-not (Test-Path -LiteralPath $target -PathType Leaf)) {
            throw "release_document_link_broken:$relative->$($match.Groups[1].Value)"
        }
    }
}

Write-Output "task4_release_docs=PASS required=$($required.Count)"
