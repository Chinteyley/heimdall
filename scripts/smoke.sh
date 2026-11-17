#!/usr/bin/env bash
# Runs every Heimdall mode against a running gateway and prints the verdicts.
set -euo pipefail
cd "$(dirname "$0")/.."
set -a; . ./.env; set +a
URL=${BIFROST_URL:-http://localhost:8080}
Q='Who is the CEO of Intel, and what is the latest stable version of React? One sentence each.'

for mode in report annotate correct check; do
  content=$Q
  [ "$mode" = check ] && content='Patrick Gelsinger is the CEO of Intel, and React 18.2 is the latest version.'
  echo "== $mode"
  curl -sS "$URL/v1/chat/completions" \
    -H 'content-type: application/json' -H "x-bf-vk: $BIFROST_VK_APP" -H "x-heimdall: $mode" \
    -d "$(jq -n --arg c "$content" '{model:"openai/gpt-4.1-mini",messages:[{role:"user",content:$c}]}')" |
    jq -r '(.choices[0].message.content // "(check mode: no answer)"), "",
           (.heimdall | "score \(.score), \(.stale) stale, \(.usage.latency_ms) ms \(.error // "")"),
           (.heimdall.claims[] | "  \(.status) \(.freshness)  \(.value) -> \(.current_value // "")")'
done
