#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
fixtures=${1:-$root/benchmarks/arena/fixtures}
manifest="$fixtures/manifest.json"

[[ -f $manifest ]] || { printf 'arena: fixture manifest missing: %s\n' "$manifest" >&2; exit 1; }

while IFS=$'\t' read -r file expected; do
  [[ -f $fixtures/$file ]] || { printf 'arena: fixture missing: %s\n' "$file" >&2; exit 1; }
  actual=$(sha256sum "$fixtures/$file" | awk '{print $1}')
  if [[ $actual != "$expected" ]]; then
    printf 'arena: fixture hash mismatch for %s: got %s, expected %s\n' "$file" "$actual" "$expected" >&2
    exit 1
  fi
done < <(jq -r 'to_entries[] | [.key, (.value | sub("^sha256:"; ""))] | @tsv' "$manifest")

printf 'arena: verified fixture hashes in %s\n' "$manifest"
