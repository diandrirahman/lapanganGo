[CmdletBinding()]
param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
)

$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
$gitRoot = (& git -C $RepoRoot rev-parse --show-toplevel 2>$null).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($gitRoot)) {
    throw "RepoRoot is not inside a Git worktree: $RepoRoot"
}

$gitRoot = (Resolve-Path -LiteralPath $gitRoot).Path
if ($gitRoot -ne $RepoRoot) {
    throw "RepoRoot must be the Git worktree root: $RepoRoot"
}

if (-not (Test-Path -LiteralPath (Join-Path $RepoRoot 'apps/api/go.mod'))) {
    throw "RepoRoot does not contain apps/api/go.mod: $RepoRoot"
}

$dirtyFiles = @(& git -C $RepoRoot status --porcelain --untracked-files=all)
if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect Git status for $RepoRoot"
}
if ($dirtyFiles.Count -gt 0) {
    throw "Backend regression requires a clean exact-commit worktree; dirty paths: $($dirtyFiles -join ', ')"
}

$testedCommit = (& git -C $RepoRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($testedCommit)) {
    throw "Unable to resolve the tested Git commit"
}

Write-Output "TASK405_COMMIT=$testedCommit"

$runToken = [Guid]::NewGuid().ToString('N')
$containerName = "lapangango_task405_go_$runToken"
$moduleVolume = "lapangango_task405_go_mod_$runToken"
$buildVolume = "lapangango_task405_go_build_$runToken"
$exitCode = 1

try {
    Write-Host "Running backend regression in Linux container $containerName..."

    docker volume create $moduleVolume | Out-Null
    docker volume create $buildVolume | Out-Null

    $dockerArgs = @(
        'run',
        '--name', $containerName,
        '--mount', "type=bind,source=$RepoRoot,target=/repo,readonly",
        '--mount', "type=volume,source=$moduleVolume,target=/go/pkg/mod",
        '--mount', "type=volume,source=$buildVolume,target=/root/.cache/go-build",
        'golang:1.26.4',
        'sh',
        '-c',
        'cd /repo/apps/api && go version && go mod download && go mod verify && go test -count=1 ./... && go vet ./...'
    )

    & docker @dockerArgs
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw "Linux backend regression exited with code $exitCode"
    }

    Write-Host 'Backend Linux regression passed.' -ForegroundColor Green
}
catch {
    Write-Error $_.Exception.Message
    $exitCode = 1
}
finally {
    $cleanupFailed = $false

    foreach ($resource in @(
        @{ Kind = 'container'; Name = $containerName; RemoveArgs = @('rm', '-f', $containerName) },
        @{ Kind = 'volume'; Name = $moduleVolume; RemoveArgs = @('volume', 'rm', $moduleVolume) },
        @{ Kind = 'volume'; Name = $buildVolume; RemoveArgs = @('volume', 'rm', $buildVolume) }
    )) {
        & docker $resource.Kind inspect $resource.Name *> $null
        $inspectExit = $LASTEXITCODE
        if ($inspectExit -eq 0) {
            & docker @($resource.RemoveArgs) *> $null
            if ($LASTEXITCODE -ne 0) {
                $cleanupFailed = $true
                Write-Host "Cleanup failed for $($resource.Kind) $($resource.Name)" -ForegroundColor Red
            }
        }
    }

    if ($cleanupFailed) {
        $exitCode = 1
    }
}

exit $exitCode
