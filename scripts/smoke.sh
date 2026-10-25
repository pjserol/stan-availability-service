#!/usr/bin/env bash
# Replays the brief's scenarios against a running service and checks every
# status code and body. Asset IDs are unique per run, so it is safe to repeat
# against a live deployment.
#
# Usage: scripts/smoke.sh http://localhost:8080
set -euo pipefail

base="${1:?usage: scripts/smoke.sh BASE_URL}"
base="${base%/}"
run="smoke-$(date +%s)"
passed=0
failed=0

# check NAME METHOD PATH BODY WANT_STATUS WANT_BODY
check() {
	local name="$1" method="$2" path="$3" body="$4" want_status="$5" want_body="$6"
	local out status got
	out="$(mktemp)"
	status="$(curl -s -o "$out" -w '%{http_code}' -X "$method" \
		-H 'Content-Type: application/json' --max-time 30 \
		${body:+--data "$body"} "$base$path")"
	got="$(cat "$out")"
	rm -f "$out"
	if [[ "$status" == "$want_status" && "$got" == "$want_body" ]]; then
		passed=$((passed + 1))
		printf '  ok    %s\n' "$name"
	else
		failed=$((failed + 1))
		printf '  FAIL  %s\n        want %s %s\n        got  %s %s\n' "$name" "$want_status" "$want_body" "$status" "$got"
	fi
}

ingest() { # ASSET INDEX URL RECEIVED_AT
	check "ingest $1 #$2" POST /ingest \
		"{\"asset_id\":\"$1\",\"segment_index\":$2,\"cdn_url\":\"$3\",\"received_at\":\"$4\"}" \
		202 '{"status":"accepted"}'
}

manifest() { # ASSET LAST_UPDATED URL... (URLs in index order from 0)
	local asset="$1" last="$2" segments="" i=0
	shift 2
	for url in "$@"; do
		segments+="${segments:+,}{\"index\":$i,\"cdn_url\":\"$url\"}"
		i=$((i + 1))
	done
	printf '{"asset_id":"%s","segments":[%s],"last_updated":"%s"}' "$asset" "$segments" "$last"
}

seg() { printf 'https://cdn.example.com/%s/seg%s.ts' "$1" "$2"; }

echo "Smoke testing $base (run $run)"

# Free-tier hosts may be asleep: give the first request time to wake them.
curl -s -o /dev/null --max-time 90 --retry 5 --retry-all-errors --retry-delay 5 "$base/health" || true
health_status="$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 "$base/health")"
if [[ "$health_status" == "200" ]]; then passed=$((passed + 1)); echo "  ok    health"; else failed=$((failed + 1)); echo "  FAIL  health: status $health_status"; fi

echo "Scenario 1: sequential"
s1="$run/thunderbirds/season/1/ep1"
ingest "$s1" 0 "$(seg s1 0)" 2024-05-01T10:00:00Z
ingest "$s1" 1 "$(seg s1 1)" 2024-05-01T10:00:01Z
ingest "$s1" 2 "$(seg s1 2)" 2024-05-01T10:00:02Z
check "manifest ordered [0,1,2]" GET "/manifest/$s1" "" 200 \
	"$(manifest "$s1" 2024-05-01T10:00:02Z "$(seg s1 0)" "$(seg s1 1)" "$(seg s1 2)")"

echo "Scenario 2: out of order"
s2="$run/scooby/season/1/ep4"
i=0
for idx in 5 2 0 3 1 4; do
	ingest "$s2" "$idx" "$(seg s2 "$idx")" "2024-05-01T10:00:0${i}Z"
	i=$((i + 1))
done
check "manifest ordered [0..5]" GET "/manifest/$s2" "" 200 \
	"$(manifest "$s2" 2024-05-01T10:00:05Z "$(seg s2 0)" "$(seg s2 1)" "$(seg s2 2)" "$(seg s2 3)" "$(seg s2 4)" "$(seg s2 5)")"

echo "Scenario 3: duplicates"
s3="$run/thetaste/season/1/ep2"
ingest "$s3" 0 "$(seg s3 0)" 2024-05-01T10:00:00Z
ingest "$s3" 1 "$(seg s3 1)" 2024-05-01T10:00:01Z
ingest "$s3" 0 "$(seg s3 0)" 2024-05-01T10:00:02Z
ingest "$s3" 1 "$(seg s3 1)" 2024-05-01T10:00:03Z
ingest "$s3" 2 "$(seg s3 2)" 2024-05-01T10:00:04Z
check "manifest exactly [0,1,2]" GET "/manifest/$s3" "" 200 \
	"$(manifest "$s3" 2024-05-01T10:00:04Z "$(seg s3 0)" "$(seg s3 1)" "$(seg s3 2)")"

echo "Scenario 4: multiple assets"
a="$run/thunderbirds/season/1/ep1-s4"
b="$run/scooby/season/1/ep4-s4"
c="$run/theoriginals/season/1/ep1-s4"
for idx in 0 1; do
	for asset in "$a" "$b" "$c"; do
		ingest "$asset" "$idx" "$(seg "${asset##*/}" "$idx")" "2024-05-01T10:00:0${idx}Z"
	done
done
for asset in "$a" "$b" "$c"; do
	check "manifest $asset is [0,1]" GET "/manifest/$asset" "" 200 \
		"$(manifest "$asset" 2024-05-01T10:00:01Z "$(seg "${asset##*/}" 0)" "$(seg "${asset##*/}" 1)")"
done

echo "Scenario 5: malformed bodies"
decode_error='{"error":"Could not decode request: JSON parsing failed"}'
check "5a invalid JSON" POST /ingest '{ "asset_id": "show/thunderbirds", "segment_index": ]]]' 400 "$decode_error"
check "5b missing cdn_url" POST /ingest \
	"{\"asset_id\":\"$run/x/ep1\",\"segment_index\":0,\"received_at\":\"2024-05-01T10:00:00Z\"}" \
	400 '{"error":"missing required field: cdn_url"}'
check "5c segment_index as string" POST /ingest \
	"{\"asset_id\":\"$run/x/ep1\",\"segment_index\":\"four\",\"cdn_url\":\"$(seg x 4)\",\"received_at\":\"2024-05-01T10:00:00Z\"}" \
	400 "$decode_error"
check "5d empty body" POST /ingest "" 400 "$decode_error"

echo "Scenario 6: asset not found"
check "unknown asset" GET "/manifest/$run/nonexistent/season/1/ep99" "" 404 '{"error":"asset not found"}'

echo
echo "$passed passed, $failed failed"
[[ "$failed" -eq 0 ]]
