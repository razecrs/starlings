import hashlib
import json
import os
import platform
import resource
import sys
import time
from pathlib import Path

import discord
from discord.state import ConnectionState

LIBRARY = "python_discord-py"
COMMIT = "65232c38702be5844cf2ce865a4777eb1928b5d0"
GUILD_ID = 41771983423143937
CHANNEL_ID = 800000000000000000
FIRST_USER = 700000000000000000
FIXTURES = Path(os.environ.get("ARENA_FIXTURES", "benchmarks/arena/fixtures"))
RAW_MESSAGE = (FIXTURES / "message-create.json").read_bytes()
RAW_GUILD = (FIXTURES / "guild-create-500.json").read_bytes()
RAW_MALFORMED = (FIXTURES / "malformed-frame.json").read_bytes()


def parse_frame(raw, event):
    frame = json.loads(raw)
    if frame.get("op") != 0 or frame.get("t") != event or not isinstance(frame.get("d"), dict):
        raise ValueError(f"not a {event} dispatch")
    return frame["d"]


class Context:
    def __init__(self, handled=False):
        self.handled = handled
        self.callbacks = 0
        self.callback_sum = 0
        self.last_message = None
        intents = discord.Intents(guilds=True, members=True, messages=True)
        # State parsing never touches HTTP; an inert sentinel makes accidental
        # network use fail loudly instead of constructing a live HTTP client.
        http = object()

        def dispatch(event, *args):
            if event == "message":
                self.last_message = args[0]
                if self.handled:
                    self.callbacks += 1
                    self.callback_sum = fnv(self.callback_sum, args[0].id + args[0].author.id)

        self.state = ConnectionState(dispatch=dispatch, handlers={}, hooks={}, http=http, intents=intents,
                                     chunk_guilds_at_startup=False, max_messages=1000)

    def message(self):
        self.state.parsers["MESSAGE_CREATE"](parse_frame(RAW_MESSAGE, "MESSAGE_CREATE"))
        message = self.last_message
        if (message is None or message.id != 1234567890123456789 or message.author.id != 222222222222222222
                or message.channel.id != 987654321098765432 or message.guild is not None
                or message.content != "hello there"):
            # No matching guild is intentionally loaded for the independent message fixture.
            raise RuntimeError("canonical message mismatch")

    def guild(self):
        self.state.parsers["GUILD_CREATE"](parse_frame(RAW_GUILD, "GUILD_CREATE"))
        guild = self.state._get_guild(GUILD_ID)
        validate_guild(guild)
        return guild


def validate_guild(guild):
    if guild is None or guild.owner_id != 80351110224678912 or len(guild.members) != 500 or len(guild.roles) != 40 or len(guild.channels) != 25:
        raise RuntimeError("guild cache mismatch")
    for offset in (0, 250, 499):
        if guild.get_member(FIRST_USER + offset) is None:
            raise RuntimeError(f"member {FIRST_USER + offset} missing")


def fnv(seed, value):
    return (seed * 1099511628211 + value) & ((1 << 64) - 1)


def digest(value):
    return "sha256:" + hashlib.sha256(str(value).encode()).hexdigest()


def workload(name):
    if name in ("message_handled", "message_unhandled"):
        context = Context(name == "message_handled")

        def run(operations):
            context.callbacks = 0
            context.callback_sum = 1469598103934665603
            for _ in range(operations):
                context.message()
            return context.callback_sum, context.callbacks

        return run, "full_dispatch", name == "message_handled"
    if name == "guild_create_state":
        context = Context()

        def run(operations):
            guild = None
            for _ in range(operations):
                guild = context.guild()
            return guild.members[0].id + guild.members[-1].id, 0

        return run, "state_update", False
    if name == "member_lookup":
        guild = Context().guild()
        ids = (FIRST_USER, FIRST_USER + 250, FIRST_USER + 499, FIRST_USER + 9999)

        def run(operations):
            total, hits = 1469598103934665603, 0
            for i in range(operations):
                member = guild.get_member(ids[i & 3])
                if member is not None:
                    hits += 1
                    total = fnv(total, member.id)
            if hits != operations - operations // 4:
                raise RuntimeError(f"lookup hits={hits}")
            return total, 0

        return run, "public_cache", False
    if name == "permission_resolve":
        guild = Context().guild()
        member = guild.get_member(FIRST_USER)
        channel = guild.get_channel(CHANNEL_ID)

        def run(operations):
            total = 1469598103934665603
            for _ in range(operations):
                value = channel.permissions_for(member).value
                if value != 76800:
                    raise RuntimeError(f"permissions={value}")
                total = fnv(total, value)
            return total, 0

        return run, "public_cache", False
    raise NotImplementedError(name)


def result(workload_name, coverage, sample, operations, elapsed, callbacks, checksum):
    return {"schema": 1, "library": LIBRARY, "language": "Python", "commit": COMMIT,
            "runtime": platform.python_version(), "build": "release", "workload": workload_name,
            "coverage": coverage, "sample": sample, "operations": operations, "elapsed_ns": elapsed,
            "ns_per_op": elapsed / operations, "peak_rss_bytes": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024,
            "callbacks": callbacks, "digest": digest(checksum)}


def verify():
    for name in ("message_handled", "message_unhandled", "guild_create_state", "member_lookup", "permission_resolve"):
        run, _, expects = workload(name)
        _, callbacks = run(1 if name == "guild_create_state" else 4)
        if callbacks != (4 if expects else 0):
            raise RuntimeError(f"{name}: callbacks={callbacks}")
    try:
        parse_frame(RAW_MALFORMED, "MESSAGE_CREATE")
    except (json.JSONDecodeError, ValueError):
        pass
    else:
        raise RuntimeError("malformed frame accepted")


def count(value, zero=False):
    number = int(value)
    if number < (0 if zero else 1):
        raise ValueError("invalid count")
    return number


def main():
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    if command == "info":
        print(json.dumps({"schema": 1, "library": LIBRARY, "language": "Python", "commit": COMMIT,
                          "runtime": platform.python_version(), "build": "release",
                          "supported": ["message_handled", "message_unhandled", "guild_create_state", "member_lookup", "permission_resolve", "malformed_frame"], "unsupported": {}}))
        return
    if command == "cold-start":
        Context()
        print(json.dumps(result("cold_start", "library_load", 0, 1, 0, 0, "ready")))
        return
    if command == "verify":
        verify()
        print(json.dumps({"schema": 1, "library": LIBRARY, "verified": True}))
        return
    if command != "bench" or len(sys.argv) != 6:
        raise ValueError("usage: main.py bench <workload> <operations> <warmup-batches> <measured-batches>")
    run, coverage, expects = workload(sys.argv[2])
    operations, warmup, measured = count(sys.argv[3]), count(sys.argv[4], True), count(sys.argv[5])
    for _ in range(warmup):
        run(operations)
    for sample in range(1, measured + 1):
        started = time.perf_counter_ns()
        checksum, callbacks = run(operations)
        elapsed = time.perf_counter_ns() - started
        if expects and callbacks != operations:
            raise RuntimeError(f"callbacks={callbacks}")
        print(json.dumps(result(sys.argv[2], coverage, sample, operations, elapsed, callbacks, checksum), separators=(",", ":")))


try:
    main()
except NotImplementedError as error:
    print(f"unsupported workload {error}", file=sys.stderr)
    sys.exit(2)
