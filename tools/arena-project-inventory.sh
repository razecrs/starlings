#!/usr/bin/env bash
set -euo pipefail

sources=${1:-/opt/arena/sources}
patterns=(
  Cargo.toml rust-toolchain.toml rust-toolchain
  go.mod go.work
  package.json pnpm-workspace.yaml yarn.lock pnpm-lock.yaml package-lock.json
  '*.csproj' '*.sln' global.json
  build.gradle build.gradle.kts settings.gradle settings.gradle.kts gradlew pom.xml
  build.sbt project/build.properties
  pubspec.yaml
  mix.exs rebar.config
  deps.edn project.clj
  shard.yml
  '*.nimble'
  composer.json
  Gemfile '*.gemspec' .ruby-version
  Project.toml
  Package.swift .swift-version
  lit.json package.lua
)

for directory in "$sources"/*; do
  [[ -d $directory/.git ]] || continue
  printf '\n[%s]\n' "${directory##*/}"
  for pattern in "${patterns[@]}"; do
    find "$directory" -maxdepth 2 -type f -name "$pattern" -printf '%P\n'
  done | sort -u
done
