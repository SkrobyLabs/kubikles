#!/usr/bin/env bash
set -euo pipefail
umask 077

test "$#" -eq 2 || exit 2
input="$1"
test_name="$2"
test -f "$input" || exit 2
[[ "$test_name" =~ ^[A-Za-z0-9_]+$ ]] || exit 2

count_action() {
  local action="$1"
  grep -F '"Action":"'"$action"'"' "$input" 2>/dev/null | grep -Fc '"Test":"'"$test_name"'"' || true
}

runs="$(count_action run)"
passes="$(count_action pass)"
skips="$(count_action skip)"
test "$runs" -eq 1
test "$passes" -eq 1
test "$skips" -eq 0
