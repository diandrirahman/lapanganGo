param(
    [string]$ApiBaseUrl = "http://localhost:8080"
)

$ErrorActionPreference = "Stop"

$ApiBaseUrl = $ApiBaseUrl.TrimEnd('/')

Write-Host "Running Smoke Tests..."

try {
    # 1. Check /health
    $health = Invoke-RestMethod -Uri "$ApiBaseUrl/health" -Method Get
    if ($health.status -ne "ok") {
        throw "/health failed, status was $($health.status)"
    }
    Write-Host "✅ /health OK" -ForegroundColor Green

    # 2. Check /db-health
    $dbHealth = Invoke-RestMethod -Uri "$ApiBaseUrl/db-health" -Method Get
    if ($dbHealth.status -ne "ok") {
        throw "/db-health failed, status was $($dbHealth.status)"
    }
    Write-Host "✅ /db-health OK" -ForegroundColor Green

    # 3. Check /venues
    $venues = Invoke-RestMethod -Uri "$ApiBaseUrl/venues" -Method Get
    if ($null -eq $venues.data) {
        throw "/venues failed: 'data' field missing (invalid paginated response)"
    }
    if ($null -eq $venues.total) {
        throw "/venues failed: 'total' field missing (invalid paginated response)"
    }
    Write-Host "✅ /venues OK" -ForegroundColor Green

    Write-Host "🎉 All smoke tests passed!" -ForegroundColor Green
    exit 0
} catch {
    Write-Host "❌ Smoke test failed: $_" -ForegroundColor Red
    exit 1
}
