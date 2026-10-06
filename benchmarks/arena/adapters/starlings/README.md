# Starlings arena adapter

This adapter uses `Client.HandleGatewayFrame`, the same typed dispatcher and
internal state consumers used by the live gateway. It never opens a network
connection.

```sh
bash benchmarks/arena/adapters/starlings/run.sh prepare
bash benchmarks/arena/adapters/starlings/run.sh verify
bash benchmarks/arena/adapters/starlings/run.sh bench message_handled 10000 5 30
```

Supported: all six canonical correctness/benchmark workloads. The adapter does
not edit Starlings or embed fixture copies.
