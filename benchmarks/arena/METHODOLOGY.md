# Arena methodology

The arena exists to answer narrow, reproducible questions. It does not turn unlike operations into one overall score.

## Comparison rules

- Every library is pinned in `source-lock.tsv`. The declared version is recorded in `libraries.tsv`.
- Fixtures are generated once and verified by SHA-256 before an adapter runs.
- Adapters use supported public library APIs. Private fields, copied internal algorithms, and substitute JSON-only work are not comparable results.
- State workloads must leave the tested object available through the library's public cache API.
- Handled dispatch must perform the library's normal decode, dispatch, and handler call. Unhandled dispatch still includes work required by internal state or generic event consumers.
- Unsupported workloads are left blank. They are never scored as zero and never silently replaced with a smaller operation.
- JIT runtimes receive warmup batches. Timed samples run on one fixed CPU set with competing adapters stopped.
- Cold start is measured in a fresh process. Steady-state workloads use 30 measured batches after warmup.
- The report includes the median, median absolute deviation, deterministic bootstrap confidence interval, and peak resident memory.

## Reproducing the run

Run the toolchain report from the repository root:

```sh
bash tools/arena-toolchain-report.sh
```

Generate and verify the fixed fixtures:

```sh
go run ./tools/arena-generate-fixtures.go
bash tools/arena-verify-fixtures.sh
```

Load the pinned environment and run the adapters:

```sh
source tools/arena-env.sh
bash tools/arena-run.sh
```

Generate the Markdown report from the raw JSONL results:

```sh
go run ./benchmarks/arena/cmd/report \
  -results /opt/arena/results/final \
  -out benchmarks/arena/results.md
```

The recorded report was produced under WSL2 on an i7-9750H. Results from another host are useful, but belong in a separate report because runtime startup, scheduling, and memory accounting change with the environment.

## Reading the result

Only rows with the same workload and coverage class are relative to one another. A fast lookup that returns a mutable cache pointer is not the same contract as a deep snapshot. Cold-start results measure runtime and library load together. Feature coverage, correctness, ergonomics, and runtime overhead remain separate evidence.
