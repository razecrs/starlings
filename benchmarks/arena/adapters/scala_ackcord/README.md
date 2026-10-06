# AckCord arena adapter

Verifies and cold-loads the pinned AckCord JVM classes. Canonical workloads are
unsupported because the real gateway path requires its actor/websocket graph;
isolated Circe decoding is deliberately not reported as gateway performance.
