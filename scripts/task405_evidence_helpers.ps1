Set-StrictMode -Version Latest

function Remove-Task405Ansi {
    param([AllowEmptyString()][string]$Text)

    return [regex]::Replace($Text, "$([char]27)\[[0-?]*[ -/]*[@-~]", '')
}

function ConvertTo-Task405SanitizedLine {
    param([AllowEmptyString()][string]$Text)

    $sanitized = $Text
    $sanitized = [regex]::Replace($sanitized, '(?i)(Authorization\s*:\s*Bearer\s+)\S+', '${1}<redacted>')
    $sanitized = [regex]::Replace($sanitized, '(?i)\b(postgres(?:ql)?|redis)://[^/\s:@]+:[^@\s/]+@', '${1}://<redacted>@')
    $sanitized = [regex]::Replace($sanitized, '(?i)("(?:password|secret|token|dsn|database_url|redis_url)"\s*:\s*")[^"]*(")', '${1}<redacted>${2}')
    $sanitized = [regex]::Replace($sanitized, '(?i)^(\s*)([A-Z0-9_]*(?:DATABASE_URL|DSN|PASSWORD|SECRET|TOKEN)|REDIS_URL)(\s*[:=]\s*).+$', '${1}${2}${3}<redacted>')
    $sanitized = [regex]::Replace($sanitized, '(?i)(\b[A-Z][A-Z0-9_]*(?:DATABASE_URL|DSN|PASSWORD|SECRET|TOKEN)\s*=)(?:"[^"]*"|''[^'']*''|\S+)', '${1}<redacted>')
    return $sanitized
}

function ConvertTo-Task405SanitizedLines {
    param([AllowEmptyCollection()][object[]]$Lines = @())

    return @($Lines | ForEach-Object {
        ConvertTo-Task405SanitizedLine -Text $_.ToString()
    })
}

function Get-Task405EvidenceCounts {
    param([AllowEmptyCollection()][object[]]$Lines = @())

    $texts = @($Lines | ForEach-Object { Remove-Task405Ansi -Text $_.ToString() })
    $goStatuses = @{}
    foreach ($text in $texts) {
        if ($text -match '^\s*---\s+(PASS|FAIL|SKIP):\s+([^\s(]+)') {
            $goStatuses[$Matches[2]] = $Matches[1]
        }
    }

    if ($goStatuses.Count -gt 0) {
        $passCount = 0
        $failCount = 0
        $skipCount = 0
        foreach ($name in $goStatuses.Keys) {
            $prefix = "$name/"
            $hasChild = @($goStatuses.Keys | Where-Object {
                $_ -ne $name -and $_.StartsWith($prefix, [System.StringComparison]::Ordinal)
            }).Count -gt 0
            if ($hasChild) {
                continue
            }
            switch ($goStatuses[$name]) {
                'PASS' { $passCount++ }
                'FAIL' { $failCount++ }
                'SKIP' { $skipCount++ }
            }
        }
        return [pscustomobject]@{ Pass = $passCount; Fail = $failCount; Skip = $skipCount }
    }

    $passCount = 0
    $failCount = 0
    $skipCount = 0
    $featureFlags = @{}
    $smokeEndpoints = @{}
    foreach ($text in $texts) {
        if ($text -match '^ok\s') {
            $passCount++
        }
        elseif ($text -match '^\?\s') {
            $skipCount++
        }
        elseif ($text -match '^\s*.*?\s+pass\s+(\d+)\s*$') {
            $passCount += [int]$Matches[1]
        }
        elseif ($text -match '^\s*.*?\s+fail\s+(\d+)\s*$') {
            $failCount += [int]$Matches[1]
        }
        elseif ($text -match '^\s*.*?\s+skipped\s+(\d+)\s*$') {
            $skipCount += [int]$Matches[1]
        }
        elseif ($text -match '^\s*Tests\s+.*?(\d+)\s+passed') {
            $passCount += [int]$Matches[1]
            if ($text -match '(\d+)\s+failed') { $failCount += [int]$Matches[1] }
            if ($text -match '(\d+)\s+skipped') { $skipCount += [int]$Matches[1] }
        }
        elseif ($text -match '^\s*platform expense form regression tests:\s*PASS\s*$') {
            $passCount++
        }
        elseif ($text -match '^\s*SUCCESS:\s*Platform finance browser QA passed\b') {
            $passCount++
        }

        if ($text -match '^\s*(PLATFORM_FINANCE_ADMIN_ENABLED|PLATFORM_MONETIZATION_ENABLED|VITE_PLATFORM_FINANCE_ADMIN_ENABLED)\s*[:=]\s*["'']?([^"''\s]+)') {
            $featureFlags[$Matches[1]] = $Matches[2]
        }
        if ($text -match '/(health|db-health|venues)\s+OK\s*$') {
            $smokeEndpoints[$Matches[1]] = $true
        }
    }

    foreach ($value in $featureFlags.Values) {
        if ($value -ceq 'false') { $passCount++ } else { $failCount++ }
    }
    $passCount += $smokeEndpoints.Count
    return [pscustomobject]@{ Pass = $passCount; Fail = $failCount; Skip = $skipCount }
}

function Get-Task405ExpectedCounts {
    param([Parameter(Mandatory = $true)][string]$GateId)

    $expectations = @{
        G05 = @{ Pass = 43; Fail = 0; Skip = 0 }
        G17 = @{ Pass = 16; Fail = 0; Skip = 0 }
        G18B = @{ Pass = 2; Fail = 0; Skip = 0 }
        G19 = @{ Pass = 3; Fail = 0; Skip = 0 }
        G20 = @{ Pass = 3; Fail = 0; Skip = 0 }
    }
    if (-not $expectations.ContainsKey($GateId)) { return $null }
    $expected = $expectations[$GateId]
    return [pscustomobject]@{ Pass = $expected.Pass; Fail = $expected.Fail; Skip = $expected.Skip }
}
