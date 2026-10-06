#!/usr/bin/env bash
set -euo pipefail

sources=${1:-/opt/arena/sources}

printf 'id\tcommit\tcommitted_at\n'
for directory in "$sources"/*; do
  [[ -d $directory/.git ]] || continue
  id=${directory##*/}
  printf '%s\t' "$id"
  git -C "$directory" show --no-patch --format='%H%x09%cI' HEAD
done
