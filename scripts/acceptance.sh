#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="$root/bin/togen"

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform is not on PATH. Run this through the flake: nix develop -c make acceptance" >&2
  exit 2
fi

export TF_PLUGIN_CACHE_DIR="${TF_PLUGIN_CACHE_DIR:-$HOME/.terraform.d/plugin-cache}"
mkdir -p "$TF_PLUGIN_CACHE_DIR"

status=0
temp_dirs=()

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
    cd "$work/infra/hcl" || exit 1
    terraform init -backend=false -input=false >/dev/null || exit 1
    terraform validate || exit 1
    terraform fmt -check -recursive -diff || exit 1
  ) || { echo "FAILED: $name"; status=1; }
  rm -rf "$work"
done
exit $status
