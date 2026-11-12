#!/usr/bin/env sh
# Smoke test of a running stack: login, profile, then one chat turn over SSE. Usage: deploy/smoke.sh [base-url]
set -eu
BASE="${1:-http://localhost:8080}"
SEED="$(dirname "$0")/../backend/seed/people.json"

PASSWORD=$(python3 -c 'import json,sys; print(next(p["account"]["password"] for p in json.load(open(sys.argv[1]))["people"] if p.get("account", {}).get("login") == "intern"))' "$SEED")

TOKEN=$(curl -fsS -X POST "$BASE/v1/public/auth/login" -H 'Content-Type: application/json' \
  -d "{\"login\":\"intern\",\"password\":\"$PASSWORD\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

curl -fsS "$BASE/v1/private/me" -H "Authorization: Bearer $TOKEN" \
  | python3 -c 'import json,sys; me=json.load(sys.stdin); assert me["user"]["login"] == "intern", me; print("smoke ok:", me["user"]["display_name"], "/", me["team"]["name"])'

curl -fsS "$BASE/v1/private/today" -H "Authorization: Bearer $TOKEN" \
  | python3 -c 'import json,sys; t=json.load(sys.stdin); assert t["total"] > 0 and t["next_steps"], t; print("smoke ok: today, day", t["day"], "done", t["done"], "of", t["total"])'
curl -fsS "$BASE/v1/private/plan" -H "Authorization: Bearer $TOKEN" \
  | python3 -c 'import json,sys; p=json.load(sys.stdin); assert len(p["items"]) == p["total"] > 0, p; print("smoke ok: plan,", p["total"], "items")'
curl -fsS "$BASE/v1/private/people?q=staging" -H "Authorization: Bearer $TOKEN" \
  | python3 -c 'import json,sys; p=json.load(sys.stdin)["people"]; assert p, p; print("smoke ok: people,", p[0]["name"])'

# Access request: submit, then the worker's advance_requests approves it (Mattermost: 15 s plus a 15 s tick).
REQ=$(curl -fsS -X POST "$BASE/v1/private/requests" -H "Authorization: Bearer $TOKEN" \
  -d '{"type_key":"access.mattermost_channel","fields":{"channel":"~messenger-team"}}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["request"]["id"])')
for _ in $(seq 1 60); do
  ST=$(curl -fsS "$BASE/v1/private/requests/$REQ" -H "Authorization: Bearer $TOKEN" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["request"]["status"])')
  [ "$ST" = "REQUEST_STATUS_PENDING" ] || break
  sleep 1
done
[ "$ST" = "REQUEST_STATUS_APPROVED" ] || { echo "request $REQ ended as $ST, want approved"; exit 1; }
echo "smoke ok: request $REQ approved"

STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/v1/private/me")
[ "$STATUS" = "401" ] || { echo "private path without token returned $STATUS, want 401"; exit 1; }

THREAD=$(curl -fsS -X POST "$BASE/v1/private/threads" -H "Authorization: Bearer $TOKEN" \
  -d '{"type":"THREAD_TYPE_GENERAL"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["thread"]["id"])')

EVENTS=$(mktemp)
trap 'rm -f "$EVENTS"' EXIT
curl -sN --max-time 90 "$BASE/v1/private/threads/$THREAD/events" -H "Authorization: Bearer $TOKEN" > "$EVENTS" &
SSE=$!
sleep 1

SENT=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/v1/private/threads/$THREAD/messages" \
  -H "Authorization: Bearer $TOKEN" -d '{"content":"Кто поможет с деплоем на staging?"}')
[ "$SENT" = "202" ] || { echo "send message returned $SENT, want 202"; exit 1; }

for _ in $(seq 1 90); do
  grep -q '^event: done' "$EVENTS" && break
  grep -q '^event: error' "$EVENTS" && break
  sleep 1
done
kill "$SSE" 2>/dev/null || true
wait "$SSE" 2>/dev/null || true
grep -q '^event: done' "$EVENTS" || { echo "no done event:"; cat "$EVENTS"; exit 1; }
echo "smoke ok: chat turn finished"
