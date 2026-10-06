'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const source = process.env.ARENA_SOURCE || '/opt/arena/work/js_discord-js';
const { Client, GatewayIntentBits } = require(path.join(source, 'packages/discord.js/src/index.js'));
const guildCreate = require(path.join(source, 'packages/discord.js/src/client/websocket/handlers/GUILD_CREATE.js'));

const library = 'js_discord-js';
const commit = '4dc1acc60497c21292ae68bdbb0a83a79d9004fe';
const firstUser = 700000000000000000n;
const guildId = '41771983423143937';
const channelId = '800000000000000000';
const fixturesDir = process.env.ARENA_FIXTURES || 'benchmarks/arena/fixtures';
const rawMessage = fs.readFileSync(path.join(fixturesDir, 'message-create.json'));
const rawGuild = fs.readFileSync(path.join(fixturesDir, 'guild-create-500.json'));
const rawMalformed = fs.readFileSync(path.join(fixturesDir, 'malformed-frame.json'));

function parseFrame(raw, type) {
  const frame = JSON.parse(raw);
  if (frame.op !== 0 || frame.t !== type || !frame.d) throw new Error(`not a ${type} dispatch`);
  return frame.d;
}

function newClient() {
  const client = new Client({ intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildMembers, GatewayIntentBits.GuildMessages] });
  client.user = { id: '999999999999999999' };
  return client;
}

function populate(client) {
  guildCreate(client, { d: parseFrame(rawGuild, 'GUILD_CREATE') }, 0);
  const guild = client.guilds.cache.get(guildId);
  if (!guild) throw new Error('guild cache miss');
  return guild;
}

function populateMessageContext(client) {
  const data = parseFrame(rawGuild, 'GUILD_CREATE');
  data.id = '111111111111111111';
  data.channels[0].id = '987654321098765432';
  guildCreate(client, { d: data }, 0);
}

function fnv(seed, value) { return BigInt.asUintN(64, seed * 1099511628211n + value); }
function digest(value) { return `sha256:${crypto.createHash('sha256').update(String(value)).digest('hex')}`; }
function base(workload, coverage, sample, operations, elapsed, callbacks, sum) {
  return { schema: 1, library, language: 'JavaScript', commit, runtime: process.version, build: 'release', workload,
    coverage, sample, operations, elapsed_ns: Number(elapsed), ns_per_op: Number(elapsed) / operations,
    peak_rss_bytes: process.memoryUsage().rss, callbacks, digest: digest(sum) };
}

function makeWorkload(name) {
  if (name === 'message_handled' || name === 'message_unhandled') {
    const client = newClient();
    populateMessageContext(client);
    let callbackCount = 0;
    let callbackSum = 0n;
    if (name === 'message_handled') client.on('messageCreate', message => {
      callbackCount++;
      callbackSum = fnv(callbackSum, BigInt(message.id) + BigInt(message.author.id));
    });
    return { coverage: 'full_dispatch', expectedCallbacks: name === 'message_handled', run(operations) {
      callbackCount = 0; callbackSum = 1469598103934665603n;
      for (let i = 0; i < operations; i++) {
        const data = parseFrame(rawMessage, 'MESSAGE_CREATE');
        // Replaying one Discord sequence number must still traverse the real action.
        client.channels.cache.get(data.channel_id)?.messages.cache.delete(data.id);
        const out = client.actions.MessageCreate.handle(data);
        if (!out.message || out.message.id !== '1234567890123456789' || out.message.author.id !== '222222222222222222' ||
            out.message.channelId !== '987654321098765432' || out.message.guildId !== '111111111111111111' || out.message.content !== 'hello there') {
          throw new Error('canonical message mismatch');
        }
      }
      return [callbackSum, callbackCount];
    }};
  }
  if (name === 'guild_create_state') {
    const client = newClient();
    return { coverage: 'state_update', expectedCallbacks: false, run(operations) {
      for (let i = 0; i < operations; i++) guildCreate(client, { d: parseFrame(rawGuild, 'GUILD_CREATE') }, 0);
      const guild = client.guilds.cache.get(guildId);
      validateGuild(guild);
      return [BigInt(guild.members.cache.first().id) + BigInt(guild.members.cache.last().id), 0];
    }};
  }
  if (name === 'member_lookup') {
    const client = newClient(); const guild = populate(client);
    const ids = [firstUser, firstUser + 250n, firstUser + 499n, firstUser + 9999n].map(String);
    return { coverage: 'public_cache', expectedCallbacks: false, run(operations) {
      let sum = 1469598103934665603n, hits = 0;
      for (let i = 0; i < operations; i++) { const member = guild.members.cache.get(ids[i & 3]); if (member) { hits++; sum = fnv(sum, BigInt(member.id)); } }
      if (hits !== operations - Math.floor(operations / 4)) throw new Error(`lookup hits=${hits}`);
      return [sum, 0];
    }};
  }
  if (name === 'permission_resolve') {
    const client = newClient(); const guild = populate(client);
    const member = guild.members.cache.get(String(firstUser)); const channel = guild.channels.cache.get(channelId);
    return { coverage: 'public_cache', expectedCallbacks: false, run(operations) {
      let sum = 1469598103934665603n;
      for (let i = 0; i < operations; i++) { const value = member.permissionsIn(channel).bitfield; if (value !== 76800n) throw new Error(`permissions=${value}`); sum = fnv(sum, value); }
      return [sum, 0];
    }};
  }
  throw Object.assign(new Error(`unsupported workload ${name}`), { unsupported: true });
}

function validateGuild(guild) {
  if (!guild || guild.ownerId !== '80351110224678912' || guild.members.cache.size !== 500 || guild.roles.cache.size !== 40 || guild.channels.cache.size !== 25) throw new Error('guild cache mismatch');
  for (const n of [0n, 250n, 499n]) if (!guild.members.cache.has(String(firstUser + n))) throw new Error(`member ${firstUser + n} missing`);
}

function verify() {
  for (const name of ['message_handled', 'message_unhandled', 'guild_create_state', 'member_lookup', 'permission_resolve']) {
    const work = makeWorkload(name); const [, callbacks] = work.run(name === 'guild_create_state' ? 1 : 4);
    if (work.expectedCallbacks && callbacks !== 4) throw new Error(`${name}: callbacks=${callbacks}`);
    if (!work.expectedCallbacks && callbacks !== 0) throw new Error(`${name}: unexpected callback`);
  }
  try { parseFrame(rawMalformed, 'MESSAGE_CREATE'); throw new Error('malformed frame accepted'); } catch (error) { if (error.message === 'malformed frame accepted') throw error; }
}

function positive(s, allowZero = false) { const n = Number(s); if (!Number.isSafeInteger(n) || n < (allowZero ? 0 : 1)) throw new Error('invalid count'); return n; }
function main() {
  const command = process.argv[2];
  if (command === 'info') return console.log(JSON.stringify({ schema: 1, library, language: 'JavaScript', commit, runtime: process.version, build: 'release', supported: ['message_handled','message_unhandled','guild_create_state','member_lookup','permission_resolve','malformed_frame'], unsupported: {} }));
  if (command === 'cold-start') { newClient(); return console.log(JSON.stringify(base('cold_start','library_load',0,1,0,0,'ready'))); }
  if (command === 'verify') { verify(); return console.log(JSON.stringify({ schema: 1, library, verified: true })); }
  if (command !== 'bench' || process.argv.length !== 7) throw new Error('usage: main.js bench <workload> <operations> <warmup-batches> <measured-batches>');
  const work = makeWorkload(process.argv[3]); const operations = positive(process.argv[4]); const warmup = positive(process.argv[5], true); const measured = positive(process.argv[6]);
  for (let i = 0; i < warmup; i++) work.run(operations);
  for (let sample = 1; sample <= measured; sample++) { const start = process.hrtime.bigint(); const [sum, callbacks] = work.run(operations); const elapsed = process.hrtime.bigint() - start; if (work.expectedCallbacks && callbacks !== operations) throw new Error(`callbacks=${callbacks}`); console.log(JSON.stringify(base(process.argv[3], work.coverage, sample, operations, elapsed, callbacks, sum))); }
}
try { main(); } catch (error) { console.error(error.stack || error.message); process.exit(error.unsupported ? 2 : 1); }
