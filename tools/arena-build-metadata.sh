#!/usr/bin/env bash
set -euo pipefail

sources=${1:-/opt/arena/sources}

for id in \
  dotnet_discord-net dotnet_dsharpplus python_discord-py lua_discordia \
  js_discord-js js_eris php_discord-php php_restcord ts_detritus; do
  printf '\n[%s root files]\n' "$id"
  find "$sources/$id" -maxdepth 1 -type f -printf '%f\n' | sort
done

printf '\n[package managers and runtime constraints]\n'
rg -n --glob 'package.json' --glob 'pubspec.yaml' --glob 'mix.exs' \
  --glob 'project.clj' --glob 'shard.yml' --glob '*.nimble' \
  --glob 'composer.json' --glob '*.gemspec' --glob 'Project.toml' \
  --glob 'Package.swift' --glob 'build.properties' --glob 'global.json' \
  'packageManager|"engines"|"scripts"|sdk:|elixir:|crystal:|nim:|php|ruby|swift-tools-version|sbt.version' \
  "$sources" || true
