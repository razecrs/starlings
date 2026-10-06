#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
install -d -m 0755 /opt/arena /opt/arena/downloads
apt-get install -y --no-install-recommends valgrind sysstat gnupg2 rsync \
  binutils libc6-dev libcurl4-openssl-dev libedit2 libicu-dev uuid-dev

download() {
  local url=$1 output=$2
  curl --fail --location --retry 3 --output "$output" "$url"
}

# Go: use the already-tested native Linux toolchain installed for this WSL user.
ln -sfn /home/raze/.local/opt/go1.27.1-snap /opt/arena/go

# Rust stable.
if [[ ! -x /opt/arena/cargo/bin/rustc ]]; then
  download https://sh.rustup.rs /opt/arena/downloads/rustup-init.sh
  RUSTUP_HOME=/opt/arena/rustup CARGO_HOME=/opt/arena/cargo \
    sh /opt/arena/downloads/rustup-init.sh -y --profile minimal --default-toolchain stable --no-modify-path
fi

# .NET 10 LTS.
if [[ ! -x /opt/arena/dotnet/dotnet ]]; then
  download https://dot.net/v1/dotnet-install.sh /opt/arena/downloads/dotnet-install.sh
  bash /opt/arena/downloads/dotnet-install.sh --channel 10.0 --install-dir /opt/arena/dotnet --no-path
fi

# Node 24 LTS. Discord.js declares Node >=24.17, so Ubuntu's Node 22 cannot be
# used for a fair HEAD build. Verify the archive against Node's own checksums.
if [[ ! -x /opt/arena/node/bin/node ]]; then
  node_base=https://nodejs.org/dist/latest-v24.x
  download "$node_base/SHASUMS256.txt" /opt/arena/downloads/node-SHASUMS256.txt
  node_archive=$(awk '$2 ~ /^node-v24\..*-linux-x64\.tar\.xz$/ {print $2}' /opt/arena/downloads/node-SHASUMS256.txt)
  if [[ -z $node_archive ]]; then
    printf 'Node 24 linux-x64 archive missing from official checksums\n' >&2
    exit 1
  fi
  download "$node_base/$node_archive" "/opt/arena/downloads/$node_archive"
  (cd /opt/arena/downloads && grep " $node_archive\$" node-SHASUMS256.txt | sha256sum --check)
  rm -rf /opt/arena/node
  install -d /opt/arena/node
  tar -xJf "/opt/arena/downloads/$node_archive" --strip-components=1 -C /opt/arena/node
fi

# Dart stable, verified against the checksum published beside the SDK archive.
dart_version=$(curl --fail --silent https://storage.googleapis.com/dart-archive/channels/stable/release/latest/VERSION | jq -r .version)
dart_base="https://storage.googleapis.com/dart-archive/channels/stable/release/${dart_version}/sdk/dartsdk-linux-x64-release.zip"
if [[ ! -x /opt/arena/dart-sdk/bin/dart ]]; then
  download "$dart_base" /opt/arena/downloads/dart.zip
  download "${dart_base}.sha256sum" /opt/arena/downloads/dart.zip.sha256sum
  dart_sha=$(awk '{print $1}' /opt/arena/downloads/dart.zip.sha256sum)
  printf '%s  %s\n' "$dart_sha" /opt/arena/downloads/dart.zip | sha256sum --check
  rm -rf /opt/arena/dart-sdk
  unzip -q /opt/arena/downloads/dart.zip -d /opt/arena
  /opt/arena/dart-sdk/bin/dart --disable-analytics >/dev/null
fi

# Julia stable from its official JSON feed, verified by the feed's SHA-256.
julia_json=/opt/arena/downloads/julia-versions.json
if [[ ! -x /opt/arena/julia/bin/julia ]]; then
  download https://julialang-s3.julialang.org/bin/versions.json "$julia_json"
  julia_version=$(jq -r '[to_entries[] | select(.value.stable == true) | .key] | sort_by(split(".") | map(tonumber)) | last' "$julia_json")
  julia_url=$(jq -r --arg v "$julia_version" '.[$v].files[] | select(.os == "linux" and .arch == "x86_64" and .kind == "archive" and .extension == "tar.gz") | .url' "$julia_json" | head -n1)
  julia_sha=$(jq -r --arg v "$julia_version" '.[$v].files[] | select(.os == "linux" and .arch == "x86_64" and .kind == "archive" and .extension == "tar.gz") | .sha256' "$julia_json" | head -n1)
  download "$julia_url" /opt/arena/downloads/julia.tar.gz
  printf '%s  %s\n' "$julia_sha" /opt/arena/downloads/julia.tar.gz | sha256sum --check
  rm -rf /opt/arena/julia
  install -d /opt/arena/julia
  tar -xzf /opt/arena/downloads/julia.tar.gz --strip-components=1 -C /opt/arena/julia
fi

# sbt, verified with the release checksum when GitHub publishes one.
if [[ ! -x /opt/arena/sbt/bin/sbt ]]; then
  sbt_release=$(curl --fail --silent https://api.github.com/repos/sbt/sbt/releases/latest)
  sbt_tag=$(jq -r .tag_name <<<"$sbt_release")
  sbt_version=${sbt_tag#v}
  download "https://github.com/sbt/sbt/releases/download/${sbt_tag}/sbt-${sbt_version}.tgz" /opt/arena/downloads/sbt.tgz
  rm -rf /opt/arena/sbt
  tar -xzf /opt/arena/downloads/sbt.tgz -C /opt/arena
fi

# Swift stable through Swift.org's official swiftly manager.
if [[ ! -x /opt/arena/swiftly/bin/swift ]]; then
  swiftly_archive=/opt/arena/downloads/swiftly-x86_64.tar.gz
  download https://download.swift.org/swiftly/linux/swiftly-x86_64.tar.gz "$swiftly_archive"
  rm -rf /opt/arena/swiftly-bootstrap /opt/arena/swiftly
  install -d /opt/arena/swiftly-bootstrap /opt/arena/swiftly
  tar -xzf "$swiftly_archive" -C /opt/arena/swiftly-bootstrap
  SWIFTLY_HOME_DIR=/opt/arena/swiftly /opt/arena/swiftly-bootstrap/swiftly init --quiet-shell-followup --assume-yes
  if [[ -d /root/.local/share/swiftly/bin && -d /root/.local/share/swiftly/toolchains ]]; then
    mv /root/.local/share/swiftly/bin /opt/arena/swiftly/bin
    mv /root/.local/share/swiftly/toolchains /opt/arena/swiftly/toolchains
  fi
fi

chmod -R a+rX /opt/arena
