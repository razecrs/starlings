# Discord library arena

This arena compares the libraries represented by the Discord API community
channels with Starlings. It runs on one WSL2 Ubuntu installation, from WSL's
native filesystem, with pinned source commits and toolchain versions.

## Rules

1. **Build health:** build and run each project's own tests at its pinned HEAD.
   If HEAD declares a runtime version, that version gets a compatibility run
   before a project is marked broken.
2. **API completeness:** score gateway, REST, interactions, components,
   sharding, state/cache, voice, rate limiting, and raw escape hatches from
   executable probes and source evidence.
3. **Correctness and safety:** replay the same gateway, REST, malformed-input,
   reconnect, rate-limit, decompression, and cache traces without contacting
   Discord.
4. **Performance:** use the same small dispatch and large guild payloads. Keep
   cold start separate from warmed steady-state throughput, latency, peak RSS,
   and allocation data. JIT runtimes receive warmups. Each timed result uses at
   least 30 measured samples and one fixed CPU set.
5. **Ergonomics:** implement equivalent minimal and production-ready bots, then
   compare required code, explicit configuration, error handling, and access to
   the underlying Discord primitives.

Runtime overhead and library quality are reported separately. Unsupported or
non-comparable measurements are labeled rather than converted into zeroes.
Every elimination must cite a reproducible result; popularity is not a score.

## Layout

- `libraries.tsv` is the canonical challenger list.
- `active.tsv` is the green-only arena roster. A library enters it only after
  its pinned source builds and its offline tests pass without compatibility
  repair.
- `fixtures/` contains generated canonical inputs plus their SHA-256 manifest.
- `adapters/<id>/run.sh` is the isolated adapter contract for one library.
- `METHODOLOGY.md` is the public measurement and fairness contract.
- `/opt/arena/sources` contains shallow source snapshots on WSL's native disk.
- `source-lock.tsv` records the exact commit and commit timestamp tested.
- `results/` contains machine-readable raw measurements and final reports.

Run the environment check from the Starlings checkout:

```sh
bash tools/arena-toolchain-report.sh
```

Regenerate fixtures deterministically from the repository root:

```sh
go run ./tools/arena-generate-fixtures.go
```

Run every adapter currently present, serially on one allowed CPU:

```sh
source tools/arena-env.sh
bash tools/arena-run.sh
```

The runner performs adapter verification first, then 30 fresh cold starts and
five warmed workloads with 30 measured batches each. It records JSONL rows and
peak RSS under `/opt/arena/results/final` by default. Generate the statistical
report with:

```sh
go run ./benchmarks/arena/cmd/report \
  -results /opt/arena/results/final \
  -out benchmarks/arena/results.md
```
