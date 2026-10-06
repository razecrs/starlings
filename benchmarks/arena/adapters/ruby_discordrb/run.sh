#!/usr/bin/env bash
set -euo pipefail
dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
src=${ARENA_SOURCE:-/opt/arena/sources/ruby_discordrb}
bundle_path=${ARENA_BUNDLE_PATH:-$HOME/.cache/starlings-arena/ruby_discordrb}
export ARENA_COMMIT=${ARENA_COMMIT:-05cd95d27c50685e663c1ce12fb7570d7a29a198}

prepare() {
  mkdir -p "$bundle_path"
  (cd "$src" && bundle config set --local path "$bundle_path" >/dev/null && \
    bundle config set --local without 'development test webhooks' >/dev/null && bundle install --jobs 4 --retry 2)
}

if [[ ${1:-} == prepare ]]; then
  prepare
  exit 0
fi
[[ -d $bundle_path/ruby ]] || prepare
cd "$src"
exec bundle exec ruby -I"$src/lib" "$dir/main.rb" "$@"
