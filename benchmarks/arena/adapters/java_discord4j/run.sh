#!/usr/bin/env bash
set -euo pipefail
d=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd); cp=/opt/arena/work/java_discord4j/gateway/build/classes/java/main
case ${1:-} in prepare) test -f "$cp/discord4j/gateway/json/dispatch/EventNames.class"; mkdir -p "$d/.classes"; javac -d "$d/.classes" "$d/Loader.java";; bench) [[ -f $d/.classes/Loader.class ]]||"$0" prepare; java -cp "$d/.classes:$cp" Loader "$@";; *) [[ -f $d/.classes/Loader.class ]]||"$0" prepare; exec java -cp "$d/.classes:$cp" Loader "$@";; esac
