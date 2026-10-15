#!/bin/sh
# The Go coverage gates (docs/adr/0011). `unit` runs the suite that needs no Docker and holds it to
# every statement, branch, condition and function; `integration` adds the tests behind the
# `integration` build tag, which need Docker, and holds the merged run to every function.
# Go measures only statements, so branches and conditions come from gobco and functions from the
# profile. Totals land in <out>/coverage-totals.json for the pull request report.
set -eu

bucket=${1:?usage: coverage.sh unit|integration [out]}
out=${2:-coverage}
mkdir -p "$out"

# Generated code is the contract's or sqlc's, pgtest and genschema are test tooling, and the
# composition root's main() is exercised by the e2e suite.
generated='/internal/platform/(oas|pg/queries|pg/pgtest)(/|$)'
# What needs Postgres to run at all: only the integration bucket reaches it.
needs_db='/(cmd/identity|internal/platform/pg|internal/users/adapters/persistence)$'

packages() {
  go list ./... | grep -Ev "$generated"
}

joined() {
  tr '\n' ',' | sed 's/,$//'
}

functions_gate() {
  go tool cover -func="$1" | grep -v '^total:' |
    grep -Ev '/cmd/identity/main\.go:[0-9]+:[[:space:]]+main[[:space:]]' >"$out/functions-$bucket.txt" || true
  uncovered=$(awk '$NF == "0.0%"' "$out/functions-$bucket.txt")
  functions=$(grep -c . "$out/functions-$bucket.txt")
  missed=$(printf '%s' "$uncovered" | grep -c . || true)
  echo "$((functions - missed)) $functions"
  if [ -n "$uncovered" ]; then
    echo "functions no test reaches:" >&2
    echo "$uncovered" >&2
    return 1
  fi
}

if [ "$bucket" = integration ]; then
  all=$(packages)
  go test -race -count=1 -tags integration -covermode=atomic \
    -coverpkg="$(echo "$all" | joined)" -coverprofile="$out/integration.out" $all
  counted=$(functions_gate "$out/integration.out")
  printf '{"integration": {"functions": {"covered": %s, "total": %s}}}\n' "${counted% *}" "${counted#* }" \
    >"$out/coverage-totals-integration.json"
  cat "$out/coverage-totals-integration.json"
  exit 0
fi

unit=$(packages | grep -Ev "$needs_db")
go test -race -count=1 -covermode=atomic -coverpkg="$(echo "$unit" | joined)" \
  -coverprofile="$out/unit.out" $unit
statements=$(go tool cover -func="$out/unit.out" | awk '/^total:/ { sub("%", "", $3); print $3 }')
counted=$(functions_gate "$out/unit.out") || exit 1
functions_run=${counted% *}
functions=${counted#* }

# gobco copies the whole module before instrumenting it, and the root node_modules is hundreds of
# megabytes of links, so it runs from a copy without it.
copy=$(mktemp -d)
trap 'rm -rf "$copy"' EXIT
rsync -a --exclude node_modules --exclude .git --exclude bin --exclude "$out" ./ "$copy/"
module=$(go list -m)

# Branch mode counts each branch taken; condition mode counts each operand of && and || both ways.
gobco_total() {
  mode=$1
  shift
  taken=0
  total=0
  for package in $unit; do
    report=$(cd "$copy" && go tool gobco "$@" "./${package#"$module"/}")
    echo "$report" >&2
    counted=$(echo "$report" | sed -n "s/^$mode coverage: \([0-9]*\)\/\([0-9]*\)\$/\1 \2/p")
    taken=$((taken + ${counted% *}))
    total=$((total + ${counted#* }))
  done
  echo "$taken $total"
}

counted=$(gobco_total Branch -branch)
branches_taken=${counted% *}
branches=${counted#* }
counted=$(gobco_total Condition)
conditions_taken=${counted% *}
conditions=${counted#* }

printf '{"statements": %s, "branches": {"covered": %s, "total": %s}, "conditions": {"covered": %s, "total": %s}, "functions": {"covered": %s, "total": %s}}\n' \
  "$statements" "$branches_taken" "$branches" "$conditions_taken" "$conditions" "$functions_run" "$functions" \
  >"$out/coverage-totals.json"
cat "$out/coverage-totals.json"

if [ "$statements" != "100.0" ] || [ "$branches_taken" != "$branches" ] ||
  [ "$conditions_taken" != "$conditions" ] || [ "$functions_run" != "$functions" ]; then
  echo "unit coverage must reach every statement, branch, condition and function" >&2
  exit 1
fi
