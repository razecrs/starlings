#!/usr/bin/env bash
set -uo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/arena-env.sh"

run() {
  local label=$1
  shift
  printf '\n[%s]\n' "$label"
  "$@" 2>&1
  local status=$?
  if ((status != 0)); then
    printf 'STATUS: FAILED (%s)\n' "$status"
    failures=$((failures + 1))
  fi
}

failures=0
run OS bash -c '. /etc/os-release; printf "%s %s (%s)\\n" "$NAME" "$VERSION_ID" "$(uname -m)"'
run Go go version
run Rust rustc --version
run Cargo cargo --version
run .NET dotnet --version
run Dart dart --version
run Julia julia --version
run Swift swift --version
run Java java -version
run Gradle gradle --version
run Maven mvn --version
run sbt sbt --script-version
run Node node --version
run npm npm --version
run Python python3 --version
run Ruby ruby --version
run Bundler bundle --version
run Crystal crystal --version
run Shards shards --version
run Nim nim --version
run Nimble nimble --version
run Elixir elixir --version
run Mix mix --version
run Clojure clojure -Sdescribe
run Leiningen lein version
run PHP php --version
run Composer composer --version
run Lua lua -v
run LuaJIT luajit -v
run Luvit luvit -v
run Lit lit --version
run Hyperfine hyperfine --version
run Valgrind valgrind --version
run GNU-time /usr/bin/time --version

printf '\nToolchain failures: %d\n' "$failures"
exit "$failures"
