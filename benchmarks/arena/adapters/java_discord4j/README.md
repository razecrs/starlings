# Discord4J arena adapter

Verifies and cold-loads the pinned gateway library. Canonical workloads are explicitly
unsupported: Discord4J does not expose a public offline raw gateway-to-core dispatch
entry point, and benchmarking its Jackson payload decoder alone would violate the arena contract.
