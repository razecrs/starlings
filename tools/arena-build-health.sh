#!/usr/bin/env bash
set -uo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/arena-env.sh"
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

sources=${ARENA_SOURCES:-/opt/arena/sources}
work_root=${ARENA_WORK:-/opt/arena/work}
results=${ARENA_RESULTS:-/opt/arena/results/build-health}
timeout_limit=${ARENA_BUILD_TIMEOUT:-30m}
mkdir -p "$work_root" "$results"

active=(
  starlings crystal_discordcr dart_nyxx dotnet_dsharpplus elixir_nostrum
  go_discordgo java_discord4j java_jda js_discord-js kotlin_kord
  lua_discordia nim_dimscord php_discord-php php_restcord
  python_discord-py ruby_discordrb scala_ackcord
)

command_for() {
  case "$1" in
    starlings|go_discordgo|go_disgord)
      printf '%s' 'go test ./...'
      ;;
    clojure_discljord)
      printf '%s' 'lein test'
      ;;
    crystal_discordcr)
      printf '%s' 'shards install && crystal spec'
      ;;
    dart_nyxx)
      printf '%s' 'dart pub get && dart test'
      ;;
    dotnet_discord-net)
      printf '%s' 'git submodule update --init --depth=1 && dotnet test Discord.Net.sln --configuration Release --nologo'
      ;;
    dotnet_dsharpplus)
      printf '%s' 'dotnet test DSharpPlus.slnx --configuration Release --nologo'
      ;;
    elixir_coxir|elixir_nostrum)
      printf '%s' 'MIX_ENV=test mix deps.get && MIX_ENV=test mix test'
      ;;
    java_discord4j|java_javacord|java_jda|kotlin_kord)
      printf '%s' './gradlew test --no-daemon --console=plain'
      ;;
    js_discord-js)
      printf '%s' 'pnpm install --frozen-lockfile && pnpm test'
      ;;
    js_eris)
      printf '%s' 'npm install --ignore-scripts && npm run lint && node -e '\''require(".")'\'''
      ;;
    julia_discord-jl)
      printf '%s' 'julia --project=. -e '\''using Pkg; Pkg.instantiate(); Pkg.test()'\'''
      ;;
    lua_discordia)
      printf '%s' 'lit install && luvit -e '\''assert(loadfile("init.lua"))'\'''
      ;;
    nim_dimscord)
      printf '%s' 'nimble install -dy && nim check dimscord.nim'
      ;;
    php_discord-php)
      printf '%s' 'composer install --no-interaction --prefer-dist && composer unit'
      ;;
    php_restcord)
      printf '%s' 'composer install --no-interaction --prefer-dist && composer validate --no-check-publish && find src -type f -name "*.php" -print0 | xargs -0 -n1 php -l'
      ;;
    python_discord-py)
      printf '%s' 'python3 -m venv .venv && .venv/bin/pip install -e '\''.[test]'\'' && .venv/bin/pytest'
      ;;
    ruby_discordrb)
      printf '%s' 'bundle config set --local path vendor/bundle && bundle install && bundle exec rspec'
      ;;
    rust_serenity|rust_twilight)
      printf '%s' 'cargo test --workspace --all-targets'
      ;;
    scala_ackcord)
      printf '%s' 'sbt -batch test'
      ;;
    swift_swiftdiscord)
      printf '%s' 'swift test'
      ;;
    ts_detritus)
      printf '%s' 'npm ci --ignore-scripts && npm run build'
      ;;
    *)
      return 1
      ;;
  esac
}

run_one() {
  local id=$1 source="$sources/$1" work="$work_root/$1"
  local log="$results/$id.log" status_file="$results/$id.status"
  local command started finished status

  if [[ ! -d $source ]]; then
    printf '[missing] %s\n' "$id"
    return 2
  fi
  if ! command=$(command_for "$id"); then
    printf '[unknown] %s\n' "$id"
    return 2
  fi
  if [[ ! -d $work ]]; then
    mkdir -p "$work"
    rsync -a "$source/" "$work/"
  fi

  printf '[build] %s: %s\n' "$id" "$command"
  started=$(date +%s)
  (
    cd "$work"
    timeout --signal=TERM --kill-after=30s "$timeout_limit" bash -lc \
      "source '$root/tools/arena-env.sh'; $command"
  ) 2>&1 | tee "$log"
  status=${PIPESTATUS[0]}
  finished=$(date +%s)
  printf 'id=%s\nstatus=%d\nduration_seconds=%d\ncommand=%s\n' \
    "$id" "$status" "$((finished - started))" "$command" >"$status_file"
  if ((status == 0)); then
    printf '[pass] %s (%ds)\n' "$id" "$((finished - started))"
  elif ((status == 124)); then
    printf '[timeout] %s (%ds)\n' "$id" "$((finished - started))"
  else
    printf '[fail] %s status=%d (%ds)\n' "$id" "$status" "$((finished - started))"
  fi
  return 0
}

if (($#)); then
  selected=("$@")
else
  selected=("${active[@]}")
fi

for id in "${selected[@]}"; do
  run_one "$id"
done
