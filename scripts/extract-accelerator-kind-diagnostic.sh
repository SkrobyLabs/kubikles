#!/usr/bin/env bash
set -euo pipefail
umask 077

test "$#" -eq 2 || exit 2
input="$1"
test_name="$2"
test -f "$input" || exit 2
test "$test_name" = TestAcceleratorIntegratedRoutingKind || exit 2

valid_rfc3339nano() {
  local value="$1"
  local pattern='^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})$'
  [[ "$value" =~ $pattern ]] || return 1
  local year=$((10#${BASH_REMATCH[1]}))
  local month=$((10#${BASH_REMATCH[2]}))
  local day=$((10#${BASH_REMATCH[3]}))
  local hour=$((10#${BASH_REMATCH[4]}))
  local minute=$((10#${BASH_REMATCH[5]}))
  local second=$((10#${BASH_REMATCH[6]}))
  local zone="${BASH_REMATCH[8]}"
  (( month >= 1 && month <= 12 && hour <= 23 && minute <= 59 && second <= 59 )) || return 1
  local month_days
  case "$month" in
    1|3|5|7|8|10|12) month_days=31 ;;
    4|6|9|11) month_days=30 ;;
    2)
      month_days=28
      if (( year % 400 == 0 || (year % 4 == 0 && year % 100 != 0) )); then
        month_days=29
      fi
      ;;
  esac
  (( day >= 1 && day <= month_days )) || return 1
  if test "$zone" != Z; then
    local zone_pattern='^[+-]([0-9]{2}):([0-9]{2})$'
    [[ "$zone" =~ $zone_pattern ]] || return 1
    local zone_hour=$((10#${BASH_REMATCH[1]}))
    local zone_minute=$((10#${BASH_REMATCH[2]}))
    (( zone_hour <= 23 && zone_minute <= 59 )) || return 1
  fi
}

mapfile -t candidates < <(grep -E 'accelerator-kind-(initial-)?diagnostic:' "$input" || true)
record_pattern='^\{"Time":"([^"]+)","Action":"output","Package":"kubikles","Test":"'"$test_name"'","Output":" +accelerator_integrated_routing_kind_test\.go:[0-9]+: accelerator-kind-diagnostic:([a-z-]+)\\n"\}$'

diagnostic=go-service-test
if test "${#candidates[@]}" -eq 1 && [[ "${candidates[0]}" =~ $record_pattern ]]; then
  timestamp="${BASH_REMATCH[1]}"
  code="${BASH_REMATCH[2]}"
  if valid_rfc3339nano "$timestamp"; then
    case "$code" in
      initial-missing|initial-sweeping|initial-resolving-zero|initial-resolving-after-provision|initial-provisioning|initial-connecting|initial-active-client-bind|initial-active-ready-path|initial-unavailable|initial-terminal|initial-unknown|initial-count-invalid|initial-client-repeat|initial-provision-retry|initial-provision-context-input|initial-provision-chart-pull|initial-provision-chart-integrity-render|initial-provision-install-conflict-permission|initial-provision-image-pull|initial-provision-job-pod|initial-provision-timeout-cancel|initial-provision-mixed|initial-provision-unknown|initial-connect-not-entered|initial-connect-tunnel|initial-connect-accelerator|initial-connect-version|initial-connect-authoritative|initial-connect-cancelled|initial-session-client-bind|initial-session-ready-path|initial-stage-mixed|stage-setup|stage-direct|stage-pre-ready|stage-ready|stage-list|stage-cancel|stage-detail|stage-watch|stage-loss|stage-resume|stage-mismatch|stage-isolation|stage-release|stage-final-verification)
        diagnostic="go-service-test-$code"
        ;;
    esac
  fi
fi
printf '%s\n' "$diagnostic"
