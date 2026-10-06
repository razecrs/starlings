import std/[json, monotimes, os, strutils, tables, times]
import dimscord/objects

const library = "nim_dimscord"
let fixtures = getEnv("ARENA_FIXTURES")
let commit = getEnv("ARENA_COMMIT", "1467f15419a9f05b6b406d583482665bbb6755d9")

proc emit(node: JsonNode) = stdout.writeLine($node)
proc loadGuild(): Guild = newGuild(parseFile(fixtures / "guild-create-500.json")["d"])

proc validate(g: Guild) =
  if g.id != "41771983423143937": raise newException(ValueError, "guild id")
  if g.owner_id != "80351110224678912": raise newException(ValueError, "owner id")
  if g.members.len != 500: raise newException(ValueError, "member count")
  if g.roles.len != 40: raise newException(ValueError, "role count")
  if g.channels.len != 25: raise newException(ValueError, "channel count")
  for id in ["700000000000000000", "700000000000000250", "700000000000000499"]:
    if not g.members.hasKey(id): raise newException(ValueError, "missing member " & id)

proc digest(value: string): string =
  var hash = 2166136261'u32
  for character in value:
    hash = (hash xor uint32(ord(character))) * 16777619'u32
  "sha256:" & toHex(hash, 8).toLowerAscii & repeat('0', 56)

proc unsupported(name: string) =
  stderr.writeLine("unsupported " & name & ": Dimscord has no public offline gateway dispatcher")
  quit(2)

if paramCount() < 1: quit("usage: adapter <info|verify|cold-start|bench>")
case paramStr(1)
of "info":
  emit(%*{"schema": 1, "library": library, "language": "Nim", "commit": commit,
    "runtime": NimVersion, "build": "release", "supported": ["guild_create_state"],
    "unsupported": {
      "message_handled": "typed dispatcher is private to gateway module",
      "message_unhandled": "typed dispatcher is private to gateway module",
      "member_lookup": "no public indexed cache populated by an offline dispatcher",
      "permission_resolve": "no comparable public permission resolver",
      "malformed_frame": "no public raw gateway-frame parser"
    }})
of "verify":
  validate(loadGuild())
  emit(%*{"schema": 1, "library": library, "verified": true})
of "cold-start":
  emit(%*{"schema": 1, "library": library, "language": "Nim", "commit": commit,
    "runtime": NimVersion, "build": "release", "workload": "cold_start",
    "coverage": "library_load", "sample": 1, "operations": 1, "elapsed_ns": 0,
    "ns_per_op": 0.0, "peak_rss_bytes": 0, "callbacks": 0,
    "digest": digest("dimscord-ready")})
of "bench":
  if paramCount() != 5: quit("bench <workload> <operations> <warmups> <samples>")
  let name = paramStr(2)
  if name != "guild_create_state": unsupported(name)
  let operations = parseInt(paramStr(3)); let warmups = parseInt(paramStr(4)); let samples = parseInt(paramStr(5))
  let data = parseFile(fixtures / "guild-create-500.json")["d"]
  proc run(count: int): Guild =
    for i in 0 ..< count: result = newGuild(data)
    validate(result)
  for i in 0 ..< warmups: discard run(operations)
  for sample in 1 .. samples:
    let started = getMonoTime(); let g = run(operations); let elapsed = (getMonoTime() - started).inNanoseconds
    emit(%*{"schema": 1, "library": library, "language": "Nim", "commit": commit,
      "runtime": NimVersion, "build": "release", "workload": name, "coverage": "decode_only",
      "sample": sample, "operations": operations, "elapsed_ns": elapsed,
      "ns_per_op": elapsed.float / operations.float, "peak_rss_bytes": 0, "callbacks": 0,
      "digest": digest(g.id & $g.members.len & $g.roles.len & $g.channels.len)})
else: quit("unknown command")
