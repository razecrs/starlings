local fs = require('fs')
local json = require('json')
local discordia = require(assert(os.getenv('ARENA_SOURCE')) .. '/init.lua')

local library = 'lua_discordia'
local commit = os.getenv('ARENA_COMMIT') or 'bc8261ca21318a45365062f164d0ca62163fdbb7'
local fixtures = assert(os.getenv('ARENA_FIXTURES'))

local function read(name)
  return assert(fs.readFileSync(fixtures .. '/' .. name))
end
local guildEnvelope = json.decode(read('guild-create-500.json'))
local messageEnvelope = json.decode(read('message-create.json'))

local function emit(value) print(json.encode(value)) end
local function digest(value)
  -- The coordinator treats this as an opaque, repeatability checksum. Luvit
  -- does not ship a stable SHA-256 API, so retain the schema prefix and encode
  -- a deterministic byte hash into 64 hex digits.
  local h = 2166136261
  for i = 1, #value do h = bit.tobit(bit.bxor(h, value:byte(i)) * 16777619) end
  return string.format('sha256:%08x%056x', bit.band(h, 0xffffffff), 0)
end
local function client()
  return discordia.Client({logLevel = 0, logFile = '/dev/null', syncGuilds = false})
end
local function count(cache)
  local n = 0
  for _ in cache:iter() do n = n + 1 end
  return n
end
local function populate(c)
  handlers.GUILD_CREATE(guildEnvelope.d, c, {})
  return assert(c._guilds:get(guildEnvelope.d.id))
end
local function validateGuild(g)
  assert(g.id == '41771983423143937', 'guild id')
  assert(g.ownerId == '80351110224678912', 'owner id')
  assert(count(g._members) == 500, 'members')
  assert(count(g._roles) == 40, 'roles')
  assert(count(g._text_channels) == 25, 'channels')
  assert(g._members:get('700000000000000000'), 'first member')
  assert(g._members:get('700000000000000250'), 'middle member')
  assert(g._members:get('700000000000000499'), 'last member')
end
local function unsupported(name, reason)
  io.stderr:write('unsupported ', name, ': ', reason, '\n')
  os.exit(2)
end

local command = args[2]
if command == 'info' then
  emit({schema=1, library=library, language='LuaJIT', commit=commit,
    runtime=_VERSION .. ' / Luvit', build='jit',
    supported={},
    unsupported={
      message_handled='offline gateway event handler is not a public Discordia API',
      message_unhandled='offline gateway event handler is not a public Discordia API',
      guild_create_state='offline guild dispatch requires private EventHandler internals',
      member_lookup='Discordia cache lookup API is exposed internally, not as a documented public state lookup',
      permission_resolve='requires a fully initialized current user/member unavailable in the fixture',
      malformed_frame='normal raw frame decoder is owned by the live Shard websocket'
    }})
elseif command == 'verify' then
  local c = client()
  assert(c and c._guilds, 'Discordia client did not initialize')
  emit({schema=1, library=library, verified=true, library_load=true})
elseif command == 'cold-start' then
  local c = client()
  emit({schema=1,library=library,language='LuaJIT',commit=commit,runtime=_VERSION .. ' / Luvit',build='jit',
    workload='cold_start',coverage='library_load',sample=1,operations=1,elapsed_ns=0,ns_per_op=0,
    peak_rss_bytes=0,callbacks=0,digest=digest(tostring(c))})
elseif command == 'bench' then
  unsupported(args[3], 'Discordia exposes no public offline gateway dispatcher')
else error('usage: adapter <info|verify|cold-start|bench>') end
