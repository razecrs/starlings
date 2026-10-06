#!/usr/bin/env bash
set -euo pipefail

. /etc/os-release
printf 'OS=%s %s ARCH=%s UID=%s\n' "$NAME" "$VERSION_ID" "$(uname -m)" "$(id -u)"
df -h / /tmp /mnt/d | tail -n +2

tools=(
  go gcc git curl wget tar unzip make cmake
  node npm python3 ruby java javac gradle dotnet cargo rustc
  swift dart julia nim crystal elixir mix clojure lein
  php composer lua luajit sbt hyperfine perf
)

for tool in "${tools[@]}"; do
  if command -v "$tool" >/dev/null 2>&1; then
    printf '%-10s ' "$tool"
    "$tool" --version 2>&1 | head -n 1 || true
  else
    printf '%-10s MISSING\n' "$tool"
  fi
done
