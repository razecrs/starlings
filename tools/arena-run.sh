#!/usr/bin/env bash
set -euo pipefail

if [[ -x $HOME/.local/go/bin/go ]]; then
  export PATH="$HOME/.local/go/bin:$PATH"
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
adapters=${ARENA_ADAPTERS:-$root/benchmarks/arena/adapters}
fixtures=${ARENA_FIXTURES:-$root/benchmarks/arena/fixtures}
results=${ARENA_BENCH_RESULTS:-/opt/arena/results/final}
warmups=${ARENA_WARMUPS:-5}
samples=${ARENA_SAMPLES:-30}
mkdir -p "$results"
bash "$root/tools/arena-verify-fixtures.sh" "$fixtures"

first_allowed_cpu() {
  local allowed first
  allowed=$(awk '/Cpus_allowed_list/ {print $2}' /proc/self/status)
  first=${allowed%%,*}
  first=${first%%-*}
  printf '%s' "$first"
}

cpu=${ARENA_CPU:-$(first_allowed_cpu)}

declare -A operations=(
  [message_handled]=10000
  [message_unhandled]=10000
  [guild_create_state]=100
  [member_lookup]=1000000
  [permission_resolve]=100000
)
workloads=(message_handled message_unhandled guild_create_state member_lookup permission_resolve)

if (($#)); then
  libraries=("$@")
else
  mapfile -t libraries < <(find "$adapters" -mindepth 2 -maxdepth 2 -name run.sh -printf '%h\n' | xargs -r -n1 basename | sort)
fi

if [[ ${ARENA_REQUIRE_ACTIVE:-0} == 1 ]]; then
  mapfile -t required < <({ printf 'starlings\n'; tail -n +2 "$root/benchmarks/arena/active.tsv" | cut -f1; } | sort -u)
  for library in "${required[@]}"; do
    if [[ ! -f $adapters/$library/run.sh ]]; then
      printf 'arena: active library %s has no adapter\n' "$library" >&2
      exit 1
    fi
  done
fi

if ((${#libraries[@]} == 0)); then
  printf 'arena: no adapters found under %s\n' "$adapters" >&2
  exit 1
fi

printf 'arena: cpu=%s warmups=%s samples=%s libraries=%s\n' "$cpu" "$warmups" "$samples" "${libraries[*]}"
export ARENA_FIXTURES="$fixtures"

for library in "${libraries[@]}"; do
  runner="$adapters/$library/run.sh"
  if [[ ! -f $runner ]]; then
    printf 'arena: missing adapter %s\n' "$library" >&2
    exit 1
  fi
  printf 'arena: prepare %s\n' "$library"
  bash "$runner" prepare
  printf 'arena: verify %s\n' "$library"
  taskset -c "$cpu" bash "$runner" verify >"$results/$library.verify.json"
  jq -e '.verified == true' "$results/$library.verify.json" >/dev/null
  taskset -c "$cpu" bash "$runner" info >"$results/$library.info.json"
  jq -e --arg library "$library" '.schema == 1 and .library == $library and (.supported | type == "array") and ((.unsupported // {}) | type == "object")' \
    "$results/$library.info.json" >/dev/null

  cold="$results/$library.cold_start.jsonl"
  : >"$cold"
  printf 'arena: cold-start %s\n' "$library"
  for sample in $(seq 1 "$samples"); do
    row=$(mktemp)
    usage=$(mktemp)
    started=$(date +%s%N)
    /usr/bin/time -f '%M' -o "$usage" taskset -c "$cpu" bash "$runner" cold-start >"$row"
    finished=$(date +%s%N)
    elapsed=$((finished - started))
    rss_bytes=$(($(cat "$usage") * 1024))
    jq -c --argjson sample "$sample" --argjson elapsed "$elapsed" --argjson rss "$rss_bytes" \
      '.sample=$sample | .elapsed_ns=$elapsed | .ns_per_op=$elapsed | .peak_rss_bytes=$rss' "$row" >>"$cold"
    rm -f "$row" "$usage"
  done
done

for workload in "${workloads[@]}"; do
  for library in "${libraries[@]}"; do
    runner="$adapters/$library/run.sh"
    output="$results/$library.$workload.jsonl"
    temporary="$results/.$library.$workload.raw"
    usage="$results/.$library.$workload.rss"
    log="$results/$library.$workload.log"
    printf 'arena: bench %s %s\n' "$library" "$workload"
    set +e
    /usr/bin/time -f '%M' -o "$usage" taskset -c "$cpu" bash "$runner" bench "$workload" "${operations[$workload]}" "$warmups" "$samples" >"$temporary" 2>"$log"
    status=$?
    set -e
    if ((status == 2)); then
      mv "$log" "$results/$library.$workload.unsupported.txt"
      rm -f "$temporary" "$usage"
      continue
    fi
    if ((status != 0)); then
      printf 'arena: %s %s failed with status %d; see %s\n' "$library" "$workload" "$status" "$log" >&2
      rm -f "$temporary" "$usage"
      exit "$status"
    fi
    rss_bytes=$(($(cat "$usage") * 1024))
    jq -c --argjson rss "$rss_bytes" '.peak_rss_bytes=$rss' "$temporary" >"$output"
    rm -f "$temporary" "$usage"
    lines=$(wc -l <"$output")
    if ((lines != samples)); then
      printf 'arena: %s %s wrote %d rows, expected %d\n' "$library" "$workload" "$lines" "$samples" >&2
      exit 1
    fi
  done
done

printf 'arena: complete; raw rows in %s\n' "$results"
