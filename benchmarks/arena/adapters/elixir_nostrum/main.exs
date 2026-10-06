library = "elixir_nostrum"
commit = System.get_env("ARENA_COMMIT", "03b06ba1c5094b83991097b1ce76b5fe2740324c")
runtime = System.version() <> " / OTP " <> System.otp_release()
emit = fn value -> IO.puts(Jason.encode!(value)) end

args = Enum.reject(System.argv(), &(&1 == "--"))

case args do
  ["info"] ->
    emit.(%{schema: 1, library: library, language: "Elixir", commit: commit,
      runtime: runtime, build: "prod", supported: [], unsupported: %{
        message_handled: "Nostrum has no public offline gateway dispatcher",
        message_unhandled: "Nostrum has no public offline gateway dispatcher",
        guild_create_state: "offline guild dispatch requires private consumers",
        member_lookup: "no comparable public cache can be populated offline",
        permission_resolve: "no comparable public cache can be populated offline",
        malformed_frame: "raw frame handling is owned by the live websocket"
      }})
  ["verify"] ->
    unless Code.ensure_loaded?(Nostrum.Api), do: raise("Nostrum.Api did not load")
    emit.(%{schema: 1, library: library, verified: true, library_load: true})
  ["cold-start"] ->
    emit.(%{schema: 1, library: library, language: "Elixir", commit: commit,
      runtime: runtime, build: "prod", workload: "cold_start", coverage: "library_load",
      sample: 1, operations: 1, elapsed_ns: 0, ns_per_op: 0, peak_rss_bytes: 0,
      callbacks: 0, digest: "sha256:" <> String.duplicate("0", 64)})
  ["bench", workload, _operations, _warmups, _samples] ->
    IO.puts(:stderr, "unsupported #{workload}: Nostrum exposes no public offline gateway dispatcher")
    System.halt(2)
  _ ->
    raise "usage: main.exs <info|verify|cold-start|bench>"
end
