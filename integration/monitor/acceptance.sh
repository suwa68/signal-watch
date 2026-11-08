#!/usr/bin/env bash
# Requires Docker Compose v2+ and a Bash host; no host Go, Python, ports, or socket mount.
set -euo pipefail
cd "$(dirname "$0")/../.."
root=$PWD
project="sw-acceptance-$(date +%s)-$$"
logs=${SIGNALWATCH_ACCEPTANCE_LOG_DIR:-"$root/.superpowers/sdd/monitor-acceptance/$project"}
mkdir -p "$logs"
# Do not inherit credentials, Compose selection, source paths, or proxy settings.
unset COMPOSE_FILE COMPOSE_PROJECT_NAME COMPOSE_PROFILES COMPOSE_ENV_FILES COMPOSE_PARALLEL_LIMIT
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY http_proxy https_proxy all_proxy no_proxy
export SIGNALWATCH_TELEGRAM_BOT_TOKEN=123456:acceptance-token
export SIGNALWATCH_TELEGRAM_CHAT_ID=-100123456
export SIGNALWATCH_SOURCES_DIR="$root/integration/monitor/sources"
export SIGNALWATCH_MONITOR_IMAGE="$project-monitor:local"
export SIGNALWATCH_FIXTURE_IMAGE="$project-fixture:local"
dc() { docker compose --env-file /dev/null -p "$project" -f compose.monitor.yaml -f integration/monitor/compose.acceptance.yaml "$@"; }
ctl() { dc exec -T fixture /fixture ctl "$1"; }
snapshot() { dc logs --no-color --no-log-prefix monitor > "$logs/monitor-current.log" 2>&1; }
owner_pid=
cleanup() {
 result=$?
 trap - EXIT INT TERM
 set +e
 if [ -n "$owner_pid" ]; then kill "$owner_pid" 2>/dev/null; wait "$owner_pid" 2>/dev/null; fi
 dc logs --no-color > "$logs/compose.log" 2>&1
 dc ps -a --format json > "$logs/containers.json" 2>&1
 ctl '{"op":"ledger"}' > "$logs/ledger.json" 2>&1
 dc down --volumes --remove-orphans --timeout 15 > "$logs/cleanup.log" 2>&1
 clean=$?
 if [ "$clean" != 0 ]; then echo "Cleanup failed for owned project $project; see $logs/cleanup.log" >&2; result=1; fi
 echo "Acceptance exit=$result evidence=$logs project=$project"
 exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
fail() { echo "FAIL: $*" >&2; exit 1; }
await_event() { ctl "{\"op\":\"await\",\"kind\":\"$1\",\"key\":\"$2\",\"after\":$3}" > /dev/null; }
cursor() { ctl '{"op":"cursor"}'; }
count_runs() { snapshot; dc exec -T fixture /fixture log-count "$1" < "$logs/monitor-current.log"; }
wait_run() {
 local end=$((SECONDS+12))
 while [ "$SECONDS" -lt "$end" ]; do
  snapshot
  if dc exec -T fixture /fixture check-run "$1" "$2" "$3" < "$logs/monitor-current.log"; then return; fi
  sleep 0.1 # bounded observation polling, never used as a completion assertion
 done
 fail "no post-barrier $3 completed run for $1"
}
page() {
 local n
 n=$(count_runs "$1")
 ctl "{\"op\":\"page\",\"key\":\"$1\",\"items\":\"$2\"}" > /dev/null
 wait_run "$1" "$n" "${3:-success}"
}
mode() { ctl "{\"op\":\"mode\",\"key\":\"$1\",\"mode\":\"$2\"}" > /dev/null; }
# The response is held after receipt. Capture a log cursor while RunOnce is
# blocked, release, and await its completion; remote receipt alone is not a mark.
deliver_held() {
 local source=$1 items=$2 item=$3 c n
 mode "$source/$item" hold
 c=$(cursor)
 ctl "{\"op\":\"page\",\"key\":\"$source\",\"items\":\"$items\"}" > /dev/null
 await_event send "$source/$item" "$c"
 n=$(count_runs "$source")
 ctl "{\"op\":\"release\",\"key\":\"$source/$item\"}" > /dev/null
 wait_run "$source" "$n" success
}
assert_ledger() { ctl "{\"op\":\"assert\",\"attempts\":$1,\"replies\":$2}"; }
start() {
 local c
 c=$(cursor)
 case "$1" in
  recreate) dc up -d --no-deps --force-recreate --no-build --pull never monitor ;;
  restart) dc restart monitor ;;
  *) dc up -d --no-deps --no-build --pull never monitor ;;
 esac
 for source in alpha beta empty; do await_event held "$source" "$c"; done
}
stop() {
 dc stop --timeout 15 monitor
 dc ps -a --format json monitor | dc exec -T fixture /fixture assert-exit 0
 snapshot
 cp "$logs/monitor-current.log" "$logs/monitor-$1.log"
}
inspect_state() {
 # The inspector itself also takes a nonblocking ownership lock, and opens
 # SQLite mode=ro. It cannot create/migrate/chown/repair the volume.
 dc --profile inspect run --rm -T --no-deps --pull never inspect inspect "$1"
}

# Missing production packaging must fail, never silently skip acceptance.
dc build monitor fixture
# A pull/build failure is not offline runtime acceptance. After this point every
# start forbids builds/pulls, and all runtime networks are internal or none.
dc --profile inspect config --format json > "$logs/compose.json"
docker compose --env-file /dev/null -p "$project" -f compose.monitor.yaml config --format json > "$logs/production-compose.json"
dc run --rm -T --no-deps --pull never --entrypoint /fixture fixture validate-config < "$logs/compose.json"
dc run --rm -T --no-deps --pull never --entrypoint /fixture fixture production-policy < "$logs/production-compose.json"
git rev-parse HEAD > "$logs/source-commit.txt"
docker compose version > "$logs/compose-version.txt"
dc up -d --no-build --pull never --wait --wait-timeout 30 fixture
a=$(ctl '{"op":"ready"}')
[ "$a" = true ] || fail 'fixture not ready'
dc exec -T fixture /fixture metadata > "$logs/platform.json"

# Fresh volume populated by the real nonroot image; silent A/B and empty baseline.
start up
dc images --format json > "$logs/images.json"
image_id=$(dc images -q monitor)
[ -n "$image_id" ] || fail 'no running production image ID'
# Live inspection must fail before opening SQLite, even with a read-only mount.
if inspect_state '{"alpha":[],"beta":[],"empty":[]}' > "$logs/live-inspect.log" 2>&1; then fail 'inspector accepted a live owner'; fi
dc exec -T fixture /fixture assert-live-inspect < "$logs/live-inspect.log"
page alpha A,B
page beta A,B
page empty ''
assert_ledger '{}' '{}'
stop baseline
inspect_state '{"alpha":["A","B"],"beta":["A","B"],"empty":[]}'

# Same named volume, recreated container: C and first post-empty E each send once.
start recreate
deliver_held alpha A,B,C C
page beta A,B
deliver_held empty E E
assert_ledger '{"alpha/C":1,"empty/E":1}' '{"alpha/C":1,"empty/E":1}'
stop recreated
inspect_state '{"alpha":["A","B","C"],"beta":["A","B"],"empty":["E"]}'

# Ordinary restart (same container) also retains committed suppression.
start restart
page alpha A,B,C
page beta A,B
page empty E
assert_ledger '{"alpha/C":1,"empty/E":1}' '{"alpha/C":1,"empty/E":1}'
stop restarted

# Failed C does not prevent D; only successfully marked D survives restart.
mode beta/C fail
start recreate
page beta A,B,C,D error
assert_ledger '{"alpha/C":1,"empty/E":1,"beta/C":1,"beta/D":1}' '{"alpha/C":1,"empty/E":1,"beta/D":1}'
stop failed-c
inspect_state '{"alpha":["A","B","C"],"beta":["A","B","D"],"empty":["E"]}'
start recreate
deliver_held beta A,B,C,D C
assert_ledger '{"alpha/C":1,"empty/E":1,"beta/C":2,"beta/D":1}' '{"alpha/C":1,"empty/E":1,"beta/C":1,"beta/D":1}'
stop retried-c
inspect_state '{"alpha":["A","B","C"],"beta":["A","B","C","D"],"empty":["E"]}'

# Alpha remains in its held HTTP request while beta makes independent progress.
start recreate
c=$(cursor)
deliver_held beta A,B,C,D,F F
ctl "{\"op\":\"assert-held\",\"key\":\"alpha\",\"after\":$c}"
# Owner rejection must precede getMe and source collection. The first owner has
# no released requests here; it cannot generate a competing getMe.
g=$(ctl '{"op":"getme-count"}')
# Launch the external CLI directly so $! identifies the process to kill/reap,
# rather than a background shell function with a potentially surviving child.
docker compose --env-file /dev/null -p "$project" -f compose.monitor.yaml -f integration/monitor/compose.acceptance.yaml run --rm -T --no-deps --pull never monitor > "$logs/second-owner.log" 2>&1 &
owner_pid=$!
end=$((SECONDS+12))
while kill -0 "$owner_pid" 2>/dev/null; do
 [ "$SECONDS" -lt "$end" ] || fail 'second owner did not terminate within 12 seconds'
 sleep 0.1
done
if wait "$owner_pid"; then owner_pid=; fail 'second owner started'; fi
owner_pid=
ctl "{\"op\":\"assert-getme\",\"count\":$g}"
dc exec -T fixture /fixture assert-owner < "$logs/second-owner.log"
stop independent-signal
await_event canceled alpha "$c"
inspect_state '{"alpha":["A","B","C"],"beta":["A","B","C","D","F"],"empty":["E"]}'

# SIGKILL after known committed state. This is ordinary committed continuity,
# not a deterministic transaction/post-send crash window or a power-loss test.
start recreate
page alpha A,B,C
page beta A,B,C,D,F
page empty E
dc kill -s SIGKILL monitor
dc ps -a --format json monitor | dc exec -T fixture /fixture assert-exit 137
inspect_state '{"alpha":["A","B","C"],"beta":["A","B","C","D","F"],"empty":["E"]}'
start recreate
page alpha A,B,C
page beta A,B,C,D,F
page empty E
assert_ledger '{"alpha/C":1,"empty/E":1,"beta/C":2,"beta/D":1,"beta/F":1}' '{"alpha/C":1,"empty/E":1,"beta/C":1,"beta/D":1,"beta/F":1}'
stop kill-recovery
inspect_state '{"alpha":["A","B","C"],"beta":["A","B","C","D","F"],"empty":["E"]}'
[ "$(dc images -q monitor)" = "$image_id" ] || fail 'production image changed during acceptance'
echo 'PASS: unchanged production image offline monitoring acceptance'
