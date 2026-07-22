[CmdletBinding()]
param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path,
    [string]$LogDirectory = ''
)

# Native Docker commands write normal progress to stderr on Windows. Keep the
# stream non-terminating and enforce failure explicitly with exit-code checks.
$ErrorActionPreference = 'Continue'
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}
$RepoRoot = (Resolve-Path -LiteralPath $RepoRoot).Path
$composeFile = Join-Path $RepoRoot 'docker-compose.yml'
$overrideFile = Join-Path $RepoRoot 'docker-compose.task-4-05.yml'
$smokeFile = Join-Path $RepoRoot 'scripts/smoke_test.ps1'

foreach ($requiredPath in @($composeFile, $overrideFile, $smokeFile)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Required smoke file is missing: $requiredPath"
    }
}

$gitRoot = (& git -C $RepoRoot rev-parse --show-toplevel 2>$null).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($gitRoot)) {
    throw "RepoRoot is not inside a Git worktree: $RepoRoot"
}
$gitRoot = (Resolve-Path -LiteralPath $gitRoot).Path
if ($gitRoot -ne $RepoRoot) {
    throw "RepoRoot must be the Git worktree root: $RepoRoot"
}

$dirtyFiles = @(& git -C $RepoRoot status --porcelain --untracked-files=all)
if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect Git status for $RepoRoot"
}
if ($dirtyFiles.Count -gt 0) {
    throw "Smoke regression requires a clean exact-commit worktree; dirty paths: $($dirtyFiles -join ', ')"
}

$testedCommit = (& git -C $RepoRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($testedCommit)) {
    throw 'Unable to resolve the tested Git commit'
}

$runToken = [Guid]::NewGuid().ToString('N')
$projectName = "lapangango-task405-$runToken"
$apiBaseUrl = 'http://127.0.0.1:18080'
$ports = @(15432, 16379, 18080, 13000, 11025, 18025)
$composeArgs = @('-p', $projectName, '-f', $composeFile, '-f', $overrideFile)
$exitCode = 1
$cleanupFailed = $false
$previousContainerPrefix = [Environment]::GetEnvironmentVariable('TASK405_CONTAINER_PREFIX', 'Process')

if ([string]::IsNullOrWhiteSpace($LogDirectory)) {
    $LogDirectory = Join-Path ([System.IO.Path]::GetTempPath()) "lapangango_task405_smoke_$runToken"
}
New-Item -ItemType Directory -Force -Path $LogDirectory | Out-Null
$composeLog = Join-Path $LogDirectory 'compose.log'
$smokeLog = Join-Path $LogDirectory 'smoke.log'
$cleanupLog = Join-Path $LogDirectory 'cleanup.log'

function Assert-PortsFree {
    param([int[]]$PortList)

    foreach ($port in $PortList) {
        $listeners = @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue)
        if ($listeners.Count -gt 0) {
            throw "Task 4-05 smoke port is already in use: $port"
        }
    }
}

function Assert-ProjectResourcesAbsent {
    param([string]$Project)

    $filters = @("label=com.docker.compose.project=$Project")
    $containers = @(& docker ps -a -q --filter $filters[0] 2>$null)
    $volumes = @(& docker volume ls -q --filter $filters[0] 2>$null)
    $networks = @(& docker network ls -q --filter $filters[0] 2>$null)
    $images = @(& docker image ls --format '{{.Repository}}:{{.Tag}}' 2>$null | Where-Object { $_ -like "$Project-*" })
    if ($containers.Count -gt 0 -or $volumes.Count -gt 0 -or $networks.Count -gt 0 -or $images.Count -gt 0) {
        throw "Task 4-05 smoke project already owns Docker resources: $Project"
    }
}

function Wait-PortsFree {
    param(
        [int[]]$PortList,
        [int]$TimeoutSeconds = 30
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        try {
            Assert-PortsFree -PortList $PortList
            return
        }
        catch {
            Start-Sleep -Seconds 1
        }
    } while ((Get-Date) -lt $deadline)

    Assert-PortsFree -PortList $PortList
}

function Get-LogHash {
    param([string]$Path)
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
}

try {
    Assert-PortsFree -PortList $ports
    Assert-ProjectResourcesAbsent -Project $projectName
    [Environment]::SetEnvironmentVariable('TASK405_CONTAINER_PREFIX', $projectName, 'Process')
    Set-Content -LiteralPath $composeLog -Encoding utf8 -Value @(
        "TASK405_COMMIT=$testedCommit"
        "TASK405_PROJECT=$projectName"
        "TASK405_PORTS=$($ports -join ',')"
    )

    $composeOutput = @(& docker compose @composeArgs up --build -d --wait *>&1)
    $composeExitCode = $LASTEXITCODE
    $composeOutput | Set-Content -LiteralPath $composeLog -Encoding utf8
    Add-Content -LiteralPath $composeLog -Encoding utf8 -Value "TASK405_COMMIT=$testedCommit"
    Add-Content -LiteralPath $composeLog -Encoding utf8 -Value "TASK405_PROJECT=$projectName"
    Add-Content -LiteralPath $composeLog -Encoding utf8 -Value "TASK405_PORTS=$($ports -join ',')"
    $composeOutput | Write-Output
    if ($composeExitCode -ne 0) {
        throw "Disposable Compose startup failed with exit code $composeExitCode"
    }

    $migration = (& docker compose @composeArgs exec -T postgres psql -U lapangango_user -d lapangango_db -Atc 'SELECT version,dirty FROM schema_migrations LIMIT 1' 2>&1 | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to read disposable migration state"
    }
    Add-Content -LiteralPath $composeLog -Encoding utf8 -Value "TASK405_MIGRATION=$migration"
    if ($migration -ne '24|f') {
        throw "Disposable Compose migration is not 24 clean: $migration"
    }

    $smokeOutput = @(& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $smokeFile -ApiBaseUrl $apiBaseUrl *>&1)
    $smokeExitCode = $LASTEXITCODE
    $smokeOutput | Set-Content -LiteralPath $smokeLog -Encoding utf8
    $smokeOutput | Write-Output
    foreach ($requiredMarker in @('/health OK', '/db-health OK', '/venues OK')) {
        if (-not (@($smokeOutput | Where-Object { $_.ToString().Contains($requiredMarker) }).Count -gt 0)) {
            throw "Read-only smoke output is missing required marker: $requiredMarker"
        }
    }
    Add-Content -LiteralPath $smokeLog -Encoding utf8 -Value "TASK405_COMMIT=$testedCommit"
    Add-Content -LiteralPath $smokeLog -Encoding utf8 -Value "TASK405_SMOKE_EXIT=$smokeExitCode"
    if ($smokeExitCode -ne 0) {
        throw "Read-only smoke failed with exit code $smokeExitCode"
    }

    $exitCode = 0
}
catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    $exitCode = 1
}
finally {
    Add-Content -LiteralPath $cleanupLog -Encoding utf8 -Value "TASK405_COMMIT=$testedCommit"
    Add-Content -LiteralPath $cleanupLog -Encoding utf8 -Value "TASK405_PROJECT=$projectName"
    # --rmi local removes only images built for this unique Compose project;
    # pre-existing images from other projects are not in scope.
    $cleanupOutput = @(& docker compose @composeArgs down -v --remove-orphans --rmi local *>&1)
    $cleanupExitCode = $LASTEXITCODE
    $cleanupOutput | Set-Content -LiteralPath $cleanupLog -Encoding utf8
    $cleanupOutput | Write-Output
    if ($cleanupExitCode -ne 0) {
        $cleanupFailed = $true
    }

    try {
        Assert-ProjectResourcesAbsent -Project $projectName
    }
    catch {
        $cleanupFailed = $true
        Add-Content -LiteralPath $cleanupLog -Encoding utf8 -Value $_.Exception.Message
    }

    try {
        Wait-PortsFree -PortList $ports
    }
    catch {
        $cleanupFailed = $true
        Add-Content -LiteralPath $cleanupLog -Encoding utf8 -Value $_.Exception.Message
    }

    if ($cleanupFailed) {
        Write-Host 'Task 4-05 smoke cleanup failed.' -ForegroundColor Red
        $exitCode = 1
    }

    foreach ($logPath in @($composeLog, $smokeLog, $cleanupLog)) {
        if (Test-Path -LiteralPath $logPath) {
            Write-Output "TASK405_LOG=$logPath"
            Write-Output "TASK405_LOG_SHA256=$(Get-LogHash -Path $logPath)"
        }
    }

    [Environment]::SetEnvironmentVariable('TASK405_CONTAINER_PREFIX', $previousContainerPrefix, 'Process')
}

exit $exitCode
