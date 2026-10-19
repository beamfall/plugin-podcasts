#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

run_contracts() {
  echo "== gate contracts =="
  python3 .github/workflows/coverage_gate_test.py
  python3 .github/workflows/gate_contract_test.py
}

run_coverage() {
  echo "== per-package coverage =="
  python3 .github/workflows/coverage_gate.py
}

run_govulncheck() {
  echo "== govulncheck (pinned tools module) =="
  if ! go -C tools tool 2>/dev/null | grep -q '(golang.org/x/vuln/cmd/govulncheck)$'; then
    echo "govulncheck: tools module does not declare golang.org/x/vuln/cmd/govulncheck" >&2
    exit 1
  fi
  go -C tools tool govulncheck -C .. ./...
}

run_fast_gate() {
  echo "== gofmt =="
  fmt_out="$(gofmt -l .)"
  if [ -n "$fmt_out" ]; then
    echo "gofmt: files need formatting:"
    echo "$fmt_out"
    exit 1
  fi

  echo "== go vet =="
  go vet ./...

  run_contracts
  run_coverage

  echo "== golangci-lint =="
  golangci-lint run ./...

  run_govulncheck

  echo "ALL GATES GREEN"
}

run_fuzz() {
  fuzz_time="${FUZZ_TIME:-30s}"
  if [[ ! "$fuzz_time" =~ ^([1-9][0-9]*)(s|x)$ ]]; then
    echo "fuzz: FUZZ_TIME must be a positive whole-second or execution budget, got $fuzz_time" >&2
    exit 1
  fi

  listed="$(go test -list '^FuzzParseOPML$' .)"
  targets="$(printf '%s\n' "$listed" | awk '/^Fuzz/ { print }')"
  if [ "$targets" != "FuzzParseOPML" ]; then
    echo "fuzz: expected exactly FuzzParseOPML, discovered: ${targets:-none}" >&2
    exit 1
  fi

  go test -run '^$' -fuzz '^FuzzParseOPML$' -fuzztime="$fuzz_time" .
}

case "${1:-gate}" in
  gate)
    run_fast_gate
    ;;
  govulncheck)
    run_govulncheck
    ;;
  contracts)
    run_contracts
    ;;
  coverage)
    run_coverage
    ;;
  race)
    echo "== race =="
    go test -race -count=1 ./...
    ;;
  fuzz)
    echo "== bounded OPML fuzz =="
    run_fuzz
    ;;
  *)
    echo "usage: $0 [gate|govulncheck|contracts|coverage|race|fuzz]" >&2
    exit 2
    ;;
esac
