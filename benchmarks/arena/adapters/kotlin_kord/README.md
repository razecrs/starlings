# Kord arena adapter

Verifies and cold-loads the pinned Kord JVM artifact. Canonical workloads are
unsupported because public gateway consumption is tied to websocket/session flows;
using kotlinx serialization directly would be a lower-level substitute.
