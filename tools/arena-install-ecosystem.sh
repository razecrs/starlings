#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get install -y --no-install-recommends maven bundler
install -d -m 0755 /opt/arena/downloads

download() {
  curl --fail --location --retry 3 --output "$2" "$1"
}

# Current native Gradle distribution with its published checksum.
if [[ ! -x /opt/arena/gradle/bin/gradle ]]; then
  gradle_json=$(curl --fail --silent https://services.gradle.org/versions/current)
  gradle_url=$(jq -r .downloadUrl <<<"$gradle_json")
  gradle_sha=$(jq -r .checksum <<<"$gradle_json")
  gradle_version=$(jq -r .version <<<"$gradle_json")
  download "$gradle_url" /opt/arena/downloads/gradle.zip
  printf '%s  %s\n' "$gradle_sha" /opt/arena/downloads/gradle.zip | sha256sum --check
  rm -rf /opt/arena/gradle
  unzip -q /opt/arena/downloads/gradle.zip -d /opt/arena
  mv "/opt/arena/gradle-${gradle_version}" /opt/arena/gradle
fi

# Discordia runs on Luvit; Lit is its package manager.
if [[ ! -x /opt/arena/luvit/luvit || ! -x /opt/arena/luvit/lit ]]; then
  install -d /opt/arena/luvit
  download https://raw.githubusercontent.com/luvit/lit/master/get-lit.sh /opt/arena/downloads/get-lit.sh
  (cd /opt/arena/luvit && sh /opt/arena/downloads/get-lit.sh)
fi

# Language-local package managers that ship bootstrap installers.
mix local.hex --force
mix local.rebar --force
gem install bundler --no-document

# Versions declared by the checked-in JavaScript workspace manifests.
source "$(dirname "${BASH_SOURCE[0]}")/arena-env.sh"
corepack enable
corepack install --global pnpm@11.25.0
corepack install --global yarn@4.1.1

chmod -R a+rX /opt/arena
