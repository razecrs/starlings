# JDA arena adapter

Verifies and cold-loads the pinned JDA classes. Canonical workloads are explicitly
unsupported because raw dispatch is coupled to a live-initialized internal `JDAImpl`;
calling `DataObject.fromJson` alone would not measure JDA event handling.
