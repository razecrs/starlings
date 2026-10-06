#!/usr/bin/env bash
set -euo pipefail
adapter_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$adapter_dir/../../../.." && pwd)
export ARENA_FIXTURES=${ARENA_FIXTURES:-$root/benchmarks/arena/fixtures}
export ARENA_SOURCE=${ARENA_SOURCE:-/opt/arena/work/python_discord-py}
export PYTHONPATH="$ARENA_SOURCE${PYTHONPATH:+:$PYTHONPATH}"
python_bin=${ARENA_PYTHON:-$ARENA_SOURCE/.venv/bin/python}
case ${1:-} in
  prepare) "$python_bin" -m py_compile "$adapter_dir/main.py"; "$python_bin" -c 'import discord' ;;
  *) exec "$python_bin" "$adapter_dir/main.py" "$@" ;;
esac
