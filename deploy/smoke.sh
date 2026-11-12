#!/usr/bin/env sh
# Smoke test of a running API: test login, then the private profile. Usage: deploy/smoke.sh [base-url]
set -eu
BASE="${1:-http://localhost:8080}"
SEED="$(dirname "$0")/../backend/seed/people.json"

PASSWORD=$(python3 -c 'import json,sys; print(next(p["account"]["password"] for p in json.load(open(sys.argv[1]))["people"] if p.get("account", {}).get("login") == "intern"))' "$SEED")

TOKEN=$(curl -fsS -X POST "$BASE/v1/public/auth/login" -H 'Content-Type: application/json' \
  -d "{\"login\":\"intern\",\"password\":\"$PASSWORD\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

curl -fsS "$BASE/v1/private/me" -H "Authorization: Bearer $TOKEN" \
  | python3 -c 'import json,sys; me=json.load(sys.stdin); assert me["user"]["login"] == "intern", me; print("smoke ok:", me["user"]["display_name"], "/", me["team"]["name"])'

STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/v1/private/me")
[ "$STATUS" = "401" ] || { echo "private path without token returned $STATUS, want 401"; exit 1; }
