#!/usr/bin/env bash
export DOTNET_ROOT=/opt/arena/dotnet
export RUSTUP_HOME=/opt/arena/rustup
export CARGO_HOME=/opt/arena/cargo
export SWIFTLY_HOME_DIR=/opt/arena/swiftly
export SWIFTLY_BIN_DIR=/opt/arena/swiftly/bin
export SWIFTLY_TOOLCHAINS_DIR=/opt/arena/swiftly/toolchains
export PATH="/opt/arena/go/bin:/opt/arena/dotnet:/opt/arena/cargo/bin:/opt/arena/node/bin:/opt/arena/dart-sdk/bin:/opt/arena/julia/bin:/opt/arena/sbt/bin:/opt/arena/swiftly/bin:/opt/arena/gradle/bin:/opt/arena/luvit:$PATH"
if [[ -x $HOME/.local/go/bin/go ]]; then
  export PATH="$HOME/.local/go/bin:$PATH"
fi
