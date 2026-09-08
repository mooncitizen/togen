#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="$root/bin/togen"

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform is not on PATH. Run this through the flake: nix develop -c just acceptance" >&2
  exit 2
fi

export TF_PLUGIN_CACHE_DIR="${TF_PLUGIN_CACHE_DIR:-$HOME/.terraform.d/plugin-cache}"
mkdir -p "$TF_PLUGIN_CACHE_DIR"

status=0
temp_dirs=()

# shellcheck disable=SC2317 # only called via the EXIT trap below
cleanup() {
  for dir in ${temp_dirs[@]+"${temp_dirs[@]}"}; do
    rm -rf "$dir" 2>/dev/null || true
  done
}

trap cleanup EXIT

for example in "$root"/examples/*/; do
  name="$(basename "$example")"
  work="$(mktemp -d)"
  temp_dirs+=("$work")
  cp -R "$example". "$work"
  echo "== $name"
  (
    cd "$work" && "$bin" generate --target hcl || exit 1
    "$bin" cost > cost.txt || { cat cost.txt; exit 1; }
    cat cost.txt
    if grep -qiw error cost.txt; then echo "togen cost reported an error"; exit 1; fi
    cd "$work/infra/hcl" || exit 1
    terraform init -backend=false -input=false >/dev/null || exit 1
    terraform validate || exit 1
    terraform fmt -check -recursive -diff || exit 1
  ) || { echo "FAILED: $name"; status=1; }
  rm -rf "$work"
done

echo "== aws-full simulate"
work="$(mktemp -d)"
temp_dirs+=("$work")
cp -R "$root"/examples/aws-full/. "$work"
(
  cd "$work" && "$bin" simulate --json > simulate.json || { cat simulate.json; exit 1; }
  # queue-1 (jobs) has no direct source injection: its rate only exists because
  # the propagation sweep ran and resolved the function-1/function-3/function-2/
  # service-1 loop with the right damping before feeding function-1 into it.
  if ! grep -A4 '"id": "queue-1"' simulate.json | grep -q '"ratePerMonth": [1-9]'; then
    echo "togen simulate did not propagate load through the loop to queue-1"
    cat simulate.json
    exit 1
  fi
  "$bin" cost > cost.txt || { cat cost.txt; exit 1; }
  if ! grep -q 'usage derived from togen/simulation.json' cost.txt; then
    echo "togen cost did not report usage derived from the example's simulation"
    cat cost.txt
    exit 1
  fi
) || { echo "FAILED: aws-full simulate"; status=1; }
rm -rf "$work"

exit $status
