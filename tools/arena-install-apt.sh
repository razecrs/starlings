#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update

packages=(
  ca-certificates curl wget git unzip zip xz-utils tar make cmake ninja-build
  build-essential pkg-config clang lld gdb time hyperfine jq ripgrep rsync
  libssl-dev libffi-dev zlib1g-dev libyaml-dev libreadline-dev libgmp-dev
  libsqlite3-dev libbz2-dev liblzma-dev libncurses-dev libxml2-dev libxslt1-dev
  libopus-dev libsodium-dev ffmpeg
  nodejs npm python3 python3-venv python3-pip
  ruby-full default-jdk
  erlang elixir
  clojure leiningen
  crystal shards nim
  php-cli php-curl php-mbstring php-xml php-zip composer
  lua5.4 luajit luarocks
)

available=()
missing=()
for package in "${packages[@]}"; do
  if apt-cache show "$package" >/dev/null 2>&1; then
    available+=("$package")
  else
    missing+=("$package")
  fi
done

printf 'Installing %d apt packages\n' "${#available[@]}"
if ((${#missing[@]})); then
  printf 'Unavailable from apt: %s\n' "${missing[*]}"
fi
apt-get install -y --no-install-recommends "${available[@]}"
