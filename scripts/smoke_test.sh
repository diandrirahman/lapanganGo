#!/bin/bash

echo "Running Smoke Tests..."

# Check /health
HEALTH_RESP=$(curl -s http://localhost:8080/health)
if ! echo "$HEALTH_RESP" | grep -q '"status"\s*:\s*"ok"'; then
  echo "❌ /health failed: $HEALTH_RESP"
  exit 1
fi
echo "✅ /health OK"

# Check /db-health
DB_HEALTH_RESP=$(curl -s http://localhost:8080/db-health)
if ! echo "$DB_HEALTH_RESP" | grep -q '"status"\s*:\s*"ok"'; then
  echo "❌ /db-health failed: $DB_HEALTH_RESP"
  exit 1
fi
echo "✅ /db-health OK"

# Check /venues
VENUES_RESP=$(curl -s http://localhost:8080/venues)
if ! echo "$VENUES_RESP" | grep -q '"data"'; then
  echo "❌ /venues failed: invalid JSON shape"
  exit 1
fi
if ! echo "$VENUES_RESP" | grep -q '"total"'; then
  echo "❌ /venues failed: missing pagination fields"
  exit 1
fi
echo "✅ /venues OK"

echo "🎉 All smoke tests passed!"
exit 0
