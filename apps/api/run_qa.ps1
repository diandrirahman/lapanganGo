# QA Walkthrough Script

Write-Host "`n--- 1. Create Open Match (Host) ---"
$createResp = Invoke-RestMethod -Uri "$env:BASE_URL/bookings/$env:BOOKING_ID/open-matches" `
    -Method POST -Headers @{ "Authorization" = "Bearer $env:HOST_TOKEN"; "Content-Type" = "application/json" } `
    -Body '{"title": "Mabar Seru E2E Rev", "description": "Latihan QA Futsal", "level": "All Levels", "max_players": 2, "price_per_player": 50000}'
$createResp | ConvertTo-Json -Depth 5
$MATCH_ID = $createResp.open_match.id
$env:MATCH_ID = $MATCH_ID

Write-Host "`n--- 2. Public List Open Matches ---"
Invoke-RestMethod -Uri "$env:BASE_URL/open-matches" | ConvertTo-Json -Depth 5

Write-Host "`n--- 3. Detail Open Match ---"
Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID" | ConvertTo-Json -Depth 5

Write-Host "`n--- 4. Join Open Match (Host tries to join) ---"
try {
    Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID/join" -Method POST -Headers @{ "Authorization" = "Bearer $env:HOST_TOKEN" }
} catch {
    Write-Host "Error details: $($_.Exception.Message)"
}

Write-Host "`n--- 5. Join Open Match (Participant 1 joins) ---"
Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID/join" -Method POST -Headers @{ "Authorization" = "Bearer $env:PART_TOKEN" } | ConvertTo-Json -Depth 5

Write-Host "`n--- 6. Join Open Match (Participant 1 tries to join again) ---"
try {
    Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID/join" -Method POST -Headers @{ "Authorization" = "Bearer $env:PART_TOKEN" }
} catch {
    Write-Host "Error details: $($_.Exception.Message)"
}

Write-Host "`n--- 7. Join Open Match (Participant 2 joins) ---"
Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID/join" -Method POST -Headers @{ "Authorization" = "Bearer $env:PART2_TOKEN" } | ConvertTo-Json -Depth 5

Write-Host "`n--- 8. Detail Open Match After Participant 2 (status = FULL) ---"
Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID" | ConvertTo-Json -Depth 5

Write-Host "`n--- 9. Leave Open Match (Participant 2 leaves) ---"
Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID/join" -Method DELETE -Headers @{ "Authorization" = "Bearer $env:PART2_TOKEN" } | ConvertTo-Json -Depth 5

Write-Host "`n--- 10. Detail Open Match After Leave (status = OPEN) ---"
Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID" | ConvertTo-Json -Depth 5

Write-Host "`n--- 11. Create Open Match from PENDING_PAYMENT Booking ---"
try {
    Invoke-RestMethod -Uri "$env:BASE_URL/bookings/$env:PENDING_BOOKING_ID/open-matches" `
    -Method POST -Headers @{ "Authorization" = "Bearer $env:HOST_TOKEN"; "Content-Type" = "application/json" } `
    -Body '{"title": "Pending Mabar", "level": "All Levels", "max_players": 2, "price_per_player": 50000}'
} catch {
    Write-Host "Error details: $($_.Exception.Message)"
}

Write-Host "`n--- 12. Cancel Source Booking (Host cancels booking) ---"
docker exec lapangango_postgres psql -U lapangango_user -d lapangango_db -c "UPDATE bookings SET status='CANCELLED' WHERE id='$env:BOOKING_ID'"

Write-Host "`n--- 13. Join Match When Source Booking NOT CONFIRMED (Participant 2 attempts) ---"
try {
    Invoke-RestMethod -Uri "$env:BASE_URL/open-matches/$MATCH_ID/join" -Method POST -Headers @{ "Authorization" = "Bearer $env:PART2_TOKEN" }
} catch {
    Write-Host "Error details: $($_.Exception.Message)"
}
