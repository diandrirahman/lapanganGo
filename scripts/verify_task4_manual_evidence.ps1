param(
    [string[]]$DocumentPaths = @(
        "docs/task_4-06_manual_qa_booking_snapshot_projection.md",
        "docs/task_4-07_manual_qa_opex_audit_auth_owner.md",
        "docs/task_4-08_manual_qa_time_reconciliation_ux.md"
    ),
    [string[]]$EvidenceDirectories = @(
        "D:/project/lapangGo_task406_evidence_20260722",
        "D:/project/lapangGo_task407_evidence_20260722",
        "D:/project/lapangGo_task408_evidence_20260722"
    )
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot

if ($DocumentPaths.Count -ne $EvidenceDirectories.Count) {
    throw "evidence_manifest_configuration_invalid"
}

$verified = 0
for ($index = 0; $index -lt $DocumentPaths.Count; $index++) {
    $document = $DocumentPaths[$index]
    if (-not [System.IO.Path]::IsPathRooted($document)) {
        $document = Join-Path $repoRoot $document
    }
    $evidenceDirectory = $EvidenceDirectories[$index]
    if (-not (Test-Path -LiteralPath $document -PathType Leaf)) {
        throw "evidence_manifest_document_missing"
    }
    if (-not (Test-Path -LiteralPath $evidenceDirectory -PathType Container)) {
        throw "evidence_manifest_directory_missing"
    }

    $content = Get-Content -LiteralPath $document -Raw
    $matches = [regex]::Matches($content, '(?m)([A-Za-z0-9][A-Za-z0-9_.-]+)=([A-Fa-f0-9]{64})')
    if ($matches.Count -eq 0) {
        throw "evidence_manifest_has_no_digests"
    }
    foreach ($match in $matches) {
        $name = $match.Groups[1].Value
        $expected = $match.Groups[2].Value.ToUpperInvariant()
        if ([System.IO.Path]::GetFileName($name) -ne $name) {
            throw "evidence_manifest_filename_invalid"
        }
        $artifact = Join-Path $evidenceDirectory $name
        if (-not (Test-Path -LiteralPath $artifact -PathType Leaf)) {
            throw "evidence_artifact_missing:$name"
        }
        $actual = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash.ToUpperInvariant()
        if ($actual -ne $expected) {
            throw "evidence_digest_mismatch:$name"
        }
        $verified++
    }
}

if ($verified -eq 0) {
    throw "evidence_manifest_verified_zero_artifacts"
}
Write-Output "manual_evidence_verified=$verified"
