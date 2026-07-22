[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'task405_evidence_helpers.ps1')

function Assert-Equal {
    param($Actual, $Expected, [string]$Label)
    if ($Actual -ne $Expected) {
        throw "$Label expected '$Expected', got '$Actual'"
    }
}

function Assert-Counts {
    param([string]$GateId, [object[]]$Lines)
    $actual = Get-Task405EvidenceCounts -Lines $Lines
    $expected = Get-Task405ExpectedCounts -GateId $GateId
    Assert-Equal $actual.Pass $expected.Pass "$GateId pass"
    Assert-Equal $actual.Fail $expected.Fail "$GateId fail"
    Assert-Equal $actual.Skip $expected.Skip "$GateId skip"
}

$escape = [char]27
Assert-Counts G05 @(
    'platform expense form regression tests: PASS',
    'ℹ pass 2',
    'ℹ fail 0',
    'ℹ skipped 0',
    'SUCCESS: Platform finance browser QA passed for desktop and mobile viewports.',
    "${escape}[2m Tests ${escape}[22m ${escape}[32m39 passed${escape}[39m (39)"
)

$g17 = @('--- PASS: TestReconciliationBoundarySuite (0.70s)')
1..16 | ForEach-Object { $g17 += "    --- PASS: TestReconciliationBoundarySuite/case_$_ (0.01s)" }
$g17 += 'ok lapangango-api/internal/platformfinance 0.70s'
Assert-Counts G17 $g17

Assert-Counts G18B @(
    '--- PASS: TestCLIReconciliationCleanIntegration (0.30s)',
    '--- PASS: TestCLIReconciliationFaultIntegration (0.30s)',
    'PASS',
    'ok lapangango-api/cmd/reconcile-platform-finance 0.60s'
)

Assert-Counts G19 @(
    'PLATFORM_FINANCE_ADMIN_ENABLED="false"',
    'PLATFORM_MONETIZATION_ENABLED="false"',
    'VITE_PLATFORM_FINANCE_ADMIN_ENABLED="false"'
)

Assert-Counts G20 @(
    '✅ /health OK',
    '✅ /db-health OK',
    '✅ /venues OK',
    '🎉 All smoke tests passed!'
)

$canaries = @(
    'postgres://task405:canary-db-password@127.0.0.1:5432/test',
    'canary-jwt-secret',
    'canary-postgres-password',
    'canary-bearer-token',
    'redis://default:canary-redis-password@127.0.0.1:6379'
)
$secretFixture = @(
    "DATABASE_URL: $($canaries[0])",
    "JWT_SECRET=$($canaries[1])",
    "POSTGRES_PASSWORD: $($canaries[2])",
    "Authorization: Bearer $($canaries[3])",
    "REDIS_URL: $($canaries[4])",
    '{"password":"canary-json-password","token":"canary-json-token"}'
)
$sanitized = (ConvertTo-Task405SanitizedLines -Lines $secretFixture) -join "`n"
foreach ($canary in $canaries + @('canary-json-password', 'canary-json-token')) {
    if ($sanitized.Contains($canary)) { throw "sanitizer leaked canary value '$canary'" }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) "lapangango-task405-runner-$([guid]::NewGuid().ToString('N'))"
$tempRepo = Join-Path $tempRoot 'repo'
$tempLogs = Join-Path $tempRoot 'logs'
$fixtureScript = Join-Path $tempRoot 'emit-fixture.ps1'
try {
    New-Item -ItemType Directory -Force -Path $tempRepo, $tempLogs | Out-Null
    Set-Content -LiteralPath (Join-Path $tempRepo 'README.md') -Encoding utf8 -Value 'task405 runner regression fixture'
    & git -C $tempRepo init --quiet
    & git -C $tempRepo config user.email 'task405-regression@example.invalid'
    & git -C $tempRepo config user.name 'Task405 Regression'
    & git -C $tempRepo add README.md
    & git -C $tempRepo commit --quiet -m fixture
    if ($LASTEXITCODE -ne 0) { throw 'unable to create temporary clean Git repository' }

    Set-Content -LiteralPath $fixtureScript -Encoding utf8 -Value @"
Write-Output 'DATABASE_URL: $($canaries[0])'
Write-Output 'JWT_SECRET: $($canaries[1])'
Write-Output 'POSTGRES_PASSWORD: $($canaries[2])'
Write-Output 'Authorization: Bearer $($canaries[3])'
Write-Output 'REDIS_URL: $($canaries[4])'
Write-Output 'PLATFORM_FINANCE_ADMIN_ENABLED="false"'
Write-Output 'PLATFORM_MONETIZATION_ENABLED="false"'
Write-Output 'VITE_PLATFORM_FINANCE_ADMIN_ENABLED="false"'
exit 0
"@

    $powershell = (Get-Process -Id $PID).Path
    & $powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'run_task405_gate.ps1') `
        -GateId G19 -RepoRoot $tempRepo -WorkingDirectory $tempRepo -LogDirectory $tempLogs `
        -Command $fixtureScript `
        -SelectedTests 'rendered backend/frontend finance flags default false'
    if ($LASTEXITCODE -ne 0) { throw "runner fixture failed with exit code $LASTEXITCODE" }

    $persistedEvidence = (
        (Get-Content -Raw -LiteralPath (Join-Path $tempLogs 'G19.log')) +
        (Get-Content -Raw -LiteralPath (Join-Path $tempLogs 'gate-metadata.jsonl'))
    )
    foreach ($canary in $canaries) {
        if ($persistedEvidence.Contains($canary)) { throw "runner evidence leaked canary value '$canary'" }
    }
    $record = Get-Content -LiteralPath (Join-Path $tempLogs 'gate-metadata.jsonl') | Select-Object -Last 1 | ConvertFrom-Json
    Assert-Equal $record.pass_count 3 'runner G19 pass'
    Assert-Equal $record.fail_count 0 'runner G19 fail'
    Assert-Equal $record.skip_count 0 'runner G19 skip'
    Assert-Equal $record.result 'PASS' 'runner G19 result'
}
finally {
    if (Test-Path -LiteralPath $tempRoot) { Remove-Item -LiteralPath $tempRoot -Recurse -Force }
}

Write-Output 'PASS: Task 4-05 evidence sanitizer and exact counter regressions'
