#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
manifest=${1:-$root/benchmarks/arena/libraries.tsv}
destination=${2:-/opt/arena/sources}
lock=${ARENA_SOURCE_LOCK:-$root/benchmarks/arena/source-lock.tsv}

if [[ ! -f $manifest ]]; then
  printf 'manifest not found: %s\n' "$manifest" >&2
  exit 1
fi
if [[ ! -f $lock ]]; then
  printf 'source lock not found: %s\n' "$lock" >&2
  exit 1
fi
if [[ ! -d $destination || ! -w $destination ]]; then
  printf 'destination must already exist and be writable: %s\n' "$destination" >&2
  exit 1
fi

tail -n +2 "$manifest" | while IFS=$'\t' read -r id language repository; do
  target="$destination/$id"
  commit=$(awk -F '\t' -v id="$id" '$1 == id { print $2; exit }' "$lock")
  if [[ ! $commit =~ ^[0-9a-f]{40}$ ]]; then
    printf '[error] %s has no exact commit in %s\n' "$id" "$lock" >&2
    exit 1
  fi
  if [[ -e $target ]]; then
    actual=$(git -C "$target" rev-parse HEAD 2>/dev/null || true)
    if [[ $actual != "$commit" ]]; then
      printf '[error] %s exists at %s, expected %s\n' "$id" "${actual:-not-a-repository}" "$commit" >&2
      exit 1
    fi
    printf '[skip] %s already pinned at %s\n' "$id" "$commit"
    continue
  fi
  printf '[clone] %s (%s) at %s\n' "$id" "$language" "$commit"
  git init --quiet "$target"
  git -C "$target" remote add origin "$repository"
  git -C "$target" fetch --quiet --depth=1 --filter=blob:none origin "$commit"
  git -C "$target" checkout --quiet --detach "$commit"
done

printf 'Cloned sources into %s\n' "$destination"
