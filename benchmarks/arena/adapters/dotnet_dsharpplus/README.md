# DSharpPlus arena adapter

Uses the pinned DSharpPlus project's internal gateway dispatch entry point, granted to
the adapter through DSharpPlus's existing `DSharpPlus.Tests` friend assembly. This
exercises normal model decoding, cache mutation, permission calculation, and event
dispatch without a socket or token. Re-deserialization is necessary because the
library destructively consumes fields from gateway payload objects.
