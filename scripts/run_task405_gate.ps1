[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$GateId,
    [Parameter(Mandatory = $true)][string]$RepoRoot,
    [Parameter(Mandatory = $true)][string]$WorkingDirectory,
    [Parameter(Mandatory = $true)][string]$LogDirectory,
    [Parameter(Mandatory = $true)][string]$Command,
    [string[]]$ArgumentList = @(),
    [string]$SelectedTests = '',
    [string]$CleanupEvidence = '',
    [string]$Notes = '',
    [switch]$FailOnSkip,
    [string[]]$EnvironmentNames = @()
)

$ErrorActionPreference = 'Continue'
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}

. (Join-Path $PSScriptRoot 'task405_evidence_helpers.ps1')
$RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
$WorkingDirectory = (Resolve-Path -LiteralPath $WorkingDirectory).Path
New-Item -ItemType Directory -Force -Path $LogDirectory | Out-Null
$logPath = Join-Path $LogDirectory "$GateId.log"
$manifestPath = Join-Path $LogDirectory 'gate-metadata.jsonl'
$commit = (& git -C $RepoRoot rev-parse HEAD 2>$null).Trim()
$dirty = @(& git -C $RepoRoot status --porcelain --untracked-files=all)
if ($LASTEXITCODE -ne 0 -or $dirty.Count -gt 0) {
    throw "Gate $GateId requires a clean worktree: $($dirty -join ', ')"
}

$started = Get-Date
$output = @()
$exitCode = 1
Push-Location $WorkingDirectory
try {
    $output = @(& $Command @ArgumentList *>&1)
    $exitCode = $LASTEXITCODE
}
catch {
    $output += $_.Exception.Message
    $exitCode = 1
}
finally {
    Pop-Location
}
$finished = Get-Date

$counts = Get-Task405EvidenceCounts -Lines $output
$passCount = $counts.Pass
$failCount = $counts.Fail
$skipCount = $counts.Skip
$expectedCounts = Get-Task405ExpectedCounts -GateId $GateId
$countMismatch = $false
if ($null -ne $expectedCounts) {
    $countMismatch = (
        $passCount -ne $expectedCounts.Pass -or
        $failCount -ne $expectedCounts.Fail -or
        $skipCount -ne $expectedCounts.Skip
    )
}

$sanitizedOutput = ConvertTo-Task405SanitizedLines -Lines $output
if ($countMismatch) {
    $sanitizedOutput += "TASK405_COUNT_MISMATCH expected=$($expectedCounts.Pass)/$($expectedCounts.Fail)/$($expectedCounts.Skip) actual=$passCount/$failCount/$skipCount"
}
if ($sanitizedOutput.Count -eq 0) {
    Set-Content -LiteralPath $logPath -Encoding utf8 -Value ''
}
else {
    $sanitizedOutput | Set-Content -LiteralPath $logPath -Encoding utf8
}

# Docker progress is written to stderr on Windows and can be surfaced as a
# NativeCommandError record even when the process exits successfully. The
# native exit code and structured test markers remain authoritative.
$effectiveExitCode = $exitCode
if ($effectiveExitCode -eq 0 -and $FailOnSkip -and $skipCount -gt 0) {
    $effectiveExitCode = 2
}
if ($effectiveExitCode -eq 0 -and $countMismatch) {
    $effectiveExitCode = 3
}
$result = if ($effectiveExitCode -eq 0 -and $failCount -eq 0) { 'PASS' } else { 'BLOCKED' }
$record = [ordered]@{
    gate_id = $GateId
    commit = $commit
    working_directory = $WorkingDirectory
    command = ConvertTo-Task405SanitizedLine -Text ((@($Command) + @($ArgumentList)) -join ' ')
    started_at = $started.ToString('o')
    finished_at = $finished.ToString('o')
    elapsed_seconds = [Math]::Round(($finished - $started).TotalSeconds, 3)
    exit_code = $effectiveExitCode
    underlying_exit_code = $exitCode
    selected_tests = ConvertTo-Task405SanitizedLine -Text $SelectedTests
    environment_names = $EnvironmentNames
    fail_on_skip = [bool]$FailOnSkip
    pass_count = $passCount
    fail_count = $failCount
    skip_count = $skipCount
    result = $result
    log = $logPath
    log_sha256 = (Get-FileHash -LiteralPath $logPath -Algorithm SHA256).Hash
    cleanup_evidence = ConvertTo-Task405SanitizedLine -Text $CleanupEvidence
    notes = ConvertTo-Task405SanitizedLine -Text $Notes
}
$record | ConvertTo-Json -Compress | Add-Content -LiteralPath $manifestPath -Encoding utf8
Write-Output "${GateId}:$result exit=$effectiveExitCode underlying=$exitCode log=$logPath sha256=$($record.log_sha256)"
exit $effectiveExitCode
