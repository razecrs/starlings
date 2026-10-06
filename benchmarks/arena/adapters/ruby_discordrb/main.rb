# frozen_string_literal: true

require 'json'
require 'discordrb'

LIBRARY = 'ruby_discordrb'
COMMIT = ENV.fetch('ARENA_COMMIT', '05cd95d27c50685e663c1ce12fb7570d7a29a198')

def emit(value)
  puts JSON.generate(value)
end

def base(workload)
  {
    schema: 1, library: LIBRARY, language: 'Ruby', commit: COMMIT,
    runtime: RUBY_DESCRIPTION, build: 'release', workload: workload,
    coverage: 'library_load', sample: 1, operations: 1, elapsed_ns: 0,
    ns_per_op: 0, peak_rss_bytes: 0, callbacks: 0,
    digest: 'sha256:' + ('0' * 64)
  }
end

case ARGV[0]
when 'info'
  emit(schema: 1, library: LIBRARY, language: 'Ruby', commit: COMMIT,
       runtime: RUBY_DESCRIPTION, build: 'release', supported: [],
       unsupported: {
         message_handled: 'discordrb has no public offline gateway dispatcher',
         message_unhandled: 'discordrb has no public offline gateway dispatcher',
         guild_create_state: 'offline guild dispatch requires private internals',
         member_lookup: 'no comparable public cache can be populated offline',
         permission_resolve: 'no comparable public cache can be populated offline',
         malformed_frame: 'raw frame handling is owned by the live websocket'
       })
when 'verify'
  raise 'discordrb did not load' unless defined?(Discordrb::Bot)
  emit(schema: 1, library: LIBRARY, verified: true, library_load: true)
when 'cold-start'
  emit(base('cold_start'))
when 'bench'
  warn "unsupported #{ARGV[1]}: discordrb exposes no public offline gateway dispatcher"
  exit 2
else
  abort 'usage: main.rb <info|verify|cold-start|bench>'
end
