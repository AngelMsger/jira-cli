#!/usr/bin/env bash
# End-to-end test for jira-cli.
#
# Default mode: start an in-repo mock Jira server and run the CLI against
# it, asserting output and exit codes.
#
# Live mode (JIRA_E2E_LIVE=1): additionally run READ-ONLY commands against
# the real server configured in .env. No write commands are issued.
set -uo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
BIN="$ROOT/bin/jira-cli"

PASS=0
FAIL=0
pass() { echo "  PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL: $1"; FAIL=$((FAIL + 1)); }

# assert_ok <description> <command...>
assert_ok() {
  local desc="$1"; shift
  if out="$("$@" 2>/dev/null)"; then
    pass "$desc"
  else
    fail "$desc (exit $?)"
  fi
}

# assert_contains <description> <needle> <command...>
assert_contains() {
  local desc="$1" needle="$2"; shift 2
  out="$("$@" 2>/dev/null)"
  if [[ "$out" == *"$needle"* ]]; then
    pass "$desc"
  else
    fail "$desc (output did not contain '$needle')"
  fi
}

# assert_err_contains <description> <needle> <command...>  (captures stderr too)
assert_err_contains() {
  local desc="$1" needle="$2"; shift 2
  out="$("$@" 2>&1)"
  if [[ "$out" == *"$needle"* ]]; then
    pass "$desc"
  else
    fail "$desc (combined output did not contain '$needle')"
  fi
}

# assert_exit <description> <expected-code> <command...>
assert_exit() {
  local desc="$1" want="$2"; shift 2
  "$@" >/dev/null 2>&1
  local got=$?
  if [[ "$got" -eq "$want" ]]; then
    pass "$desc"
  else
    fail "$desc (exit $got, want $want)"
  fi
}

echo "==> building jira-cli"
# Pin a release-like version so the update check exercises real comparison.
LDFLAGS="-X github.com/angelmsger/jira-cli/pkg/constants.Version=0.0.1"
go build -ldflags "$LDFLAGS" -o "$BIN" ./cmd/jira-cli || { echo "build failed"; exit 1; }

echo "==> starting mock Jira server"
MOCK_LOG="$(mktemp)"
go run ./test/mockserver >"$MOCK_LOG" 2>/dev/null &
MOCK_PID=$!
trap 'kill "$MOCK_PID" 2>/dev/null' EXIT

MOCK_URL=""
for _ in $(seq 1 50); do
  MOCK_URL="$(head -n1 "$MOCK_LOG" 2>/dev/null)"
  [[ -n "$MOCK_URL" ]] && break
  sleep 0.1
done
if [[ -z "$MOCK_URL" ]]; then
  echo "mock server did not start"; exit 1
fi
echo "    mock server at $MOCK_URL"

export JIRA_SERVER="$MOCK_URL"
export JIRA_FLAVOR="datacenter"
export JIRA_PERSONAL_ACCESS_TOKEN="e2e-token"
# Point the release-update check at the mock server, not the real GitHub API.
export JIRA_RELEASE_API="$MOCK_URL/releases/latest"
TMPCFG="$(mktemp -d)"
CLI=("$BIN" --config "$TMPCFG")

echo "==> mock e2e checks"
assert_contains  "version"                "jira-cli" "${CLI[@]}" version
assert_contains  "doctor healthy"         '"healthy": true' "${CLI[@]}" doctor
assert_contains  "doctor reports update"  '"available": true' "${CLI[@]}" doctor
assert_contains  "doctor --no-update-check skips it" '"healthy": true' \
                                          "${CLI[@]}" doctor --no-update-check
assert_contains  "whoami"                 "Alice Example"  "${CLI[@]}" whoami
assert_contains  "project list"           "Engineering"    "${CLI[@]}" project list
assert_contains  "project list table"     "ENG"            "${CLI[@]}" project list --format table
assert_contains  "project list --query"   "Operations"     "${CLI[@]}" project list --query ops
assert_contains  "project get"            "Engineering"    "${CLI[@]}" project get ENG
assert_contains  "project components"     "PaaS"           "${CLI[@]}" project components ENG
assert_contains  "project versions"       "1.0.0"          "${CLI[@]}" project versions ENG
assert_contains  "project issuetypes"     "Bug"            "${CLI[@]}" project issuetypes ENG
assert_contains  "project statuses"       "In Progress"    "${CLI[@]}" project statuses ENG
assert_contains  "priority list"          "High"           "${CLI[@]}" priority list
assert_exit      "label list on DC -> 2"  2                "${CLI[@]}" label list
assert_err_contains "label list DC error code" "LABEL_LIST_DC" \
                                          "${CLI[@]}" label list
assert_contains  "field list"             '"options_count"' \
                                          "${CLI[@]}" field list --project ENG --type Bug
assert_exit      "field list needs --type -> 2" 2          "${CLI[@]}" field list --project ENG
assert_contains  "field options by id"    "PaaS"           "${CLI[@]}" field options components --project ENG
assert_contains  "field options annotates types" '"issue_types"' \
                                          "${CLI[@]}" field options components --project ENG
assert_contains  "field options by name + type" "Critical" \
                                          "${CLI[@]}" field options Severity --project ENG --type Bug
assert_exit      "field options unknown field -> 6" 6      "${CLI[@]}" field options bogus --project ENG
assert_contains  "issue get"              "Welcome"        "${CLI[@]}" issue get ENG-1
assert_contains  "issue get has components" "PaaS"         "${CLI[@]}" issue get ENG-1
assert_contains  "issue get by url"       "Welcome"        "${CLI[@]}" issue get "$MOCK_URL/browse/ENG-1"
assert_contains  "fields projection"      '"key"'          "${CLI[@]}" issue get ENG-1 --fields key,summary
assert_contains  "search raw jql"         "Welcome"        "${CLI[@]}" issue search 'project = ENG'
assert_contains  "search by flags"        "Welcome"        "${CLI[@]}" issue search --project ENG --assignee me
assert_contains  "search first page cursor" '"next"'       "${CLI[@]}" issue search --project ENG --limit 2
assert_contains  "search --all paginates" "Third issue"    "${CLI[@]}" issue search --project ENG --limit 2 --all
assert_contains  "issue create"           "ENG-100"        "${CLI[@]}" issue create --project ENG --type Task --summary "Spec"
assert_contains  "issue create dry-run"   '"dry_run": true' \
                                          "${CLI[@]}" issue create --project ENG --type Task --summary "X" --dry-run
assert_exit      "issue create no summary -> 2" 2          "${CLI[@]}" issue create --project ENG --type Task
assert_contains  "issue edit"             '"key"'          "${CLI[@]}" issue edit ENG-1 --summary "Renamed"
assert_exit      "issue edit no changes -> 2" 2            "${CLI[@]}" issue edit ENG-1
assert_contains  "issue assign"           "assigned"       "${CLI[@]}" issue assign ENG-1 --to alice
assert_contains  "issue unassign"         "assigned"       "${CLI[@]}" issue assign ENG-1 --unassign
assert_contains  "issue transitions"      "Start Progress" "${CLI[@]}" issue transitions ENG-1
assert_contains  "issue transition by name" '"key"'        "${CLI[@]}" issue transition ENG-1 --to "Start Progress"
assert_contains  "issue transition by id" '"key"'          "${CLI[@]}" issue transition ENG-1 --to 21
assert_exit      "unknown transition -> 6" 6               "${CLI[@]}" issue transition ENG-1 --to "Reopen"
assert_contains  "transition dry-run"     '"dry_run": true' \
                                          "${CLI[@]}" issue transition ENG-1 --to 21 --dry-run
assert_contains  "comment list"           "First comment"  "${CLI[@]}" comment list ENG-1
assert_contains  "comment add"            "looks good"     "${CLI[@]}" comment add ENG-1 --body "looks good"
assert_contains  "comment update"         "revised"        "${CLI[@]}" comment update 10001 --issue ENG-1 --body "revised"
assert_exit      "comment delete needs --yes -> 2" 2       "${CLI[@]}" comment delete 10001 --issue ENG-1 </dev/null
assert_contains  "comment delete --yes"   "deleted"        "${CLI[@]}" comment delete 10001 --issue ENG-1 --yes
assert_contains  "comment delete dry-run" '"dry_run": true' \
                                          "${CLI[@]}" comment delete 10001 --issue ENG-1 --dry-run
assert_contains  "user resolve (DC passthrough)" "alice"   "${CLI[@]}" user resolve alice
SKILL_DIR="$(mktemp -d)"
assert_contains  "skill install"          '"installed"' \
                                          "${CLI[@]}" skill install --dir "$SKILL_DIR"
assert_contains  "skill install --agent codex" '"codex"' \
                                          env HOME="$(mktemp -d)" "${CLI[@]}" skill install --agent codex
assert_contains  "skill uninstall"        '"removed"' \
                                          "${CLI[@]}" skill uninstall --dir "$SKILL_DIR"
assert_contains  "skill uninstall (repeat)" '"not_installed"' \
                                          "${CLI[@]}" skill uninstall --dir "$SKILL_DIR"
assert_contains  "skill show"             "name: jira" "${CLI[@]}" skill show
assert_exit      "missing issue -> 6"     6                "${CLI[@]}" issue get ENG-404
assert_exit      "bad flag -> 2"          2                "${CLI[@]}" issue get ENG-1 --bogus
assert_exit      "unknown subcommand -> 2" 2               "${CLI[@]}" issue frobnicate
assert_err_contains "unknown subcommand suggests" "UNKNOWN_COMMAND" \
                                          "${CLI[@]}" config use-contexts

# Read-only mode: env JIRA_CLI_READ_ONLY blocks writes; --allow-writes
# overrides it; --dry-run remains usable.
RO_ENV=(env JIRA_CLI_READ_ONLY=1)
assert_err_contains "read-only blocks issue create"  "READONLY_BLOCKED" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" issue create --project ENG --type Task --summary "X"
assert_err_contains "read-only error names --allow-writes" "--allow-writes" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" issue create --project ENG --type Task --summary "X"
assert_exit         "read-only exit category=permission -> 5" 5 \
                                                     "${RO_ENV[@]}" "${CLI[@]}" comment delete 10001 --issue ENG-1 --yes
assert_contains     "read-only + --dry-run still previews" '"method": "DELETE"' \
                                                     "${RO_ENV[@]}" "${CLI[@]}" comment delete 10001 --issue ENG-1 --yes --dry-run
assert_contains     "--allow-writes overrides read-only"   "deleted" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" --allow-writes comment delete 10001 --issue ENG-1 --yes
assert_err_contains "read-only blocks comment add"   "READONLY_BLOCKED" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" comment add ENG-1 --body "x"

echo "==> multi-context checks"
TMPCFG2="$(mktemp -d)"
cat >"$TMPCFG2/config.yaml" <<EOF
current_context: default
contexts:
  - name: default
    server: $MOCK_URL
    flavor: datacenter
    auth: {scheme: pat}
  - name: alt
    server: $MOCK_URL
    flavor: datacenter
    auth: {scheme: pat}
defaults:
  format: json
EOF
CLI2=("$BIN" --config "$TMPCFG2")
assert_contains  "get-contexts lists default" "default"      "${CLI2[@]}" config get-contexts
assert_contains  "get-contexts lists alt"     "alt"          "${CLI2[@]}" config get-contexts
assert_ok        "use-context alt"                           "${CLI2[@]}" config use-context alt
assert_exit      "unknown context -> 3"       3              "${CLI2[@]}" --use-context ghost doctor
assert_contains  "--use-context selects ctx"  '"healthy": true' \
                                              "${CLI2[@]}" --use-context default doctor
assert_contains  "config show exposes context" '"context"'  "${CLI2[@]}" config show
assert_ok        "delete-context alt"                        "${CLI2[@]}" config delete-context alt
assert_exit      "delete last context -> 2"   2              "${CLI2[@]}" config delete-context default

if [[ "${JIRA_E2E_LIVE:-0}" == "1" ]]; then
  echo "==> live read-only checks (real server from .env)"
  unset JIRA_SERVER JIRA_FLAVOR JIRA_PERSONAL_ACCESS_TOKEN
  LIVECLI=("$BIN" --config "$(mktemp -d)")
  assert_ok "live doctor"       "${LIVECLI[@]}" doctor
  assert_ok "live project list" "${LIVECLI[@]}" project list --limit 1
fi

echo
echo "==> e2e summary: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
