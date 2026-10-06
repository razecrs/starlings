# Nyxx arena adapter

Verifies Nyxx's public `EventParser` and malformed-frame behavior. Typed event
creation and cache mutation require a complete `NyxxGateway` client graph, whose
normal construction performs Discord REST discovery. Warm workloads therefore
exit 2 instead of benchmarking only `jsonDecode` or fabricated managers.
