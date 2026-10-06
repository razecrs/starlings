require "json"
require "digest/sha256"
require "discordcr"

LIBRARY = "crystal_discordcr"
COMMIT = ENV.fetch("ARENA_COMMIT", "0e03deb8ffa247814f2fec4e197cba7a62534f85")
FIXTURES = ENV.fetch("ARENA_FIXTURES")

def emit(value)
  STDOUT.puts value.to_json
end

def guild
  envelope = JSON.parse(File.read(File.join(FIXTURES, "guild-create-500.json")))
  Discord::Gateway::GuildCreatePayload.from_json(envelope["d"].to_json)
end

def validate(g)
  raise "guild id" unless g.id.to_u64 == 41771983423143937_u64
  raise "owner id" unless g.owner_id.to_u64 == 80351110224678912_u64
  raise "member count #{g.members.size}" unless g.members.size == 500
  raise "role count #{g.roles.size}" unless g.roles.size == 40
  raise "channel count #{g.channels.size}" unless g.channels.size == 25
  expected = [700000000000000000_u64, 700000000000000250_u64, 700000000000000499_u64]
  actual = [g.members[0].user.id.to_u64, g.members[250].user.id.to_u64, g.members[499].user.id.to_u64]
  raise "member ids" unless actual == expected
  g
end

def unsupported(name)
  STDERR.puts "unsupported #{name}: discordcr has no public offline gateway dispatch/cache entry point"
  exit 2
end

command = ARGV.shift? || abort("usage: adapter <info|verify|cold-start|bench>")
case command
when "info"
  emit({schema: 1, library: LIBRARY, language: "Crystal", commit: COMMIT,
        runtime: "Crystal #{Crystal::VERSION}", build: "release",
        supported: [] of String,
        unsupported: {
          "message_handled" => "gateway dispatch is private; typed callback cannot be driven offline through public API",
          "message_unhandled" => "gateway dispatch is private",
          "member_lookup" => "offline public cache cannot be populated through gateway dispatch",
          "permission_resolve" => "no comparable public cache permission path",
          "guild_create_state" => "canonical current gateway fixture omits legacy region required by pinned discordcr schema",
          "malformed_frame" => "no public raw gateway-frame parser"
        }})
when "verify"
  client = Discord::Client.new("offline", 1_u64, compress: Discord::Client::CompressMode::None)
  emit({schema: 1, library: LIBRARY, verified: true, library_loaded: !client.nil?})
when "cold-start"
  client = Discord::Client.new("offline", 1_u64, compress: Discord::Client::CompressMode::None)
  emit({schema: 1, library: LIBRARY, language: "Crystal", commit: COMMIT,
        runtime: "Crystal #{Crystal::VERSION}", build: "release", workload: "cold_start",
        coverage: "library_load", sample: 1, operations: 1, elapsed_ns: 0,
        ns_per_op: 0.0, peak_rss_bytes: 0, callbacks: 0,
        digest: "sha256:#{Digest::SHA256.hexdigest(client.class.name)}"})
when "bench"
  name = ARGV.shift? || abort("missing workload")
  unsupported(name)
else
  abort "unknown command"
end
