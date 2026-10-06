using System.Diagnostics;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using DSharpPlus;
using DSharpPlus.Entities;
using DSharpPlus.Net.Abstractions;
using Newtonsoft.Json;

const string Library = "dotnet_dsharpplus";
const string Commit = "fd82340af43a921f49f6196194c574d77cf26510";
const ulong GuildId = 41771983423143937;
const ulong ChannelId = 800000000000000000;
const ulong FirstUser = 700000000000000000;

static void Write(object value) => Console.WriteLine(System.Text.Json.JsonSerializer.Serialize(value));
static string Digest(ulong value) => "sha256:" + Convert.ToHexStringLower(SHA256.HashData(Encoding.UTF8.GetBytes(value.ToString())));
static GatewayPayload Parse(byte[] bytes) => JsonConvert.DeserializeObject<GatewayPayload>(Encoding.UTF8.GetString(bytes)) ?? throw new InvalidDataException("null gateway payload");
static DiscordClient Client(Action<DSharpPlus.EventHandlingBuilder>? events = null)
{
    var builder = DiscordClientBuilder.CreateDefault("offline", DiscordIntents.AllUnprivileged).DisableDefaultLogging();
    if (events is not null) builder.ConfigureEventHandlers(events);
    return builder.Build();
}

var fixtures = Environment.GetEnvironmentVariable("ARENA_FIXTURES") ?? "benchmarks/arena/fixtures";
if (args.Length == 0) throw new ArgumentException("command required");
if (args[0] == "info")
{
    Write(new { schema = 1, library = Library, language = "C#", commit = Commit, runtime = System.Runtime.InteropServices.RuntimeInformation.FrameworkDescription, build = "Release", supported = new[] { "message_handled", "message_unhandled", "guild_create_state", "member_lookup", "permission_resolve", "malformed_frame" } });
    return;
}
if (args[0] == "cold-start")
{
    _ = Client();
    Write(new { schema = 1, library = Library, language = "C#", commit = Commit, runtime = System.Runtime.InteropServices.RuntimeInformation.FrameworkDescription, build = "Release", workload = "cold_start", coverage = "library_load", sample = 1, operations = 1, elapsed_ns = 0, ns_per_op = 0, peak_rss_bytes = Process.GetCurrentProcess().PeakWorkingSet64, callbacks = 0, digest = Digest(0) });
    return;
}

byte[] message = File.ReadAllBytes(Path.Combine(fixtures, "message-create.json"));
byte[] guild = File.ReadAllBytes(Path.Combine(fixtures, "guild-create-500.json"));
byte[] malformed = File.ReadAllBytes(Path.Combine(fixtures, "malformed-frame.json"));

async Task Verify()
{
    ulong callbacks = 0;
    var client = Client(e => e.HandleMessageCreated((_, ev) =>
    {
        if (ev.Message.Id != 1234567890123456789 || ev.Message.Author.Id != 222222222222222222 || ev.Message.ChannelId != 987654321098765432 || ev.Message.Content != "hello there")
            throw new InvalidDataException("canonical message mismatch");
        callbacks++;
        return Task.CompletedTask;
    }));
    await client.HandleDispatchAsync(Parse(guild));
    client.guilds[111111111111111111] = client.guilds[GuildId];
    await client.HandleDispatchAsync(Parse(message));
    if (callbacks != 1) throw new InvalidDataException($"callbacks={callbacks}");
    var state = Client();
    await state.HandleDispatchAsync(Parse(guild));
    var g = state.Guilds[GuildId];
    if (g.OwnerId != 80351110224678912 || g.Members.Count != 500 || g.Roles.Count != 40 || g.Channels.Count != 25) throw new InvalidDataException($"guild cache mismatch owner={g.OwnerId} members={g.Members.Count} roles={g.Roles.Count} channels={g.Channels.Count}");
    foreach (var id in new[] { FirstUser, FirstUser + 250, FirstUser + 499 }) if (!g.Members.ContainsKey(id)) throw new InvalidDataException($"missing member {id}");
    var permissions = ulong.Parse(g.Channels[ChannelId].PermissionsFor(g.Members[FirstUser]).ToString());
    if (permissions != 76800) throw new InvalidDataException($"permissions={permissions}");
    try { _ = Parse(malformed); throw new InvalidDataException("malformed frame accepted"); } catch (JsonSerializationException) { }
}

if (args[0] == "verify")
{
    await Verify();
    Write(new { schema = 1, library = Library, verified = true });
    return;
}
if (args[0] != "bench" || args.Length != 5) throw new ArgumentException("bench <workload> <operations> <warmup-batches> <measured-batches>");
string workload = args[1];
int operations = int.Parse(args[2]), warmups = int.Parse(args[3]), samples = int.Parse(args[4]);

Func<Task<(ulong sum, ulong callbacks, string coverage)>> Run;
if (workload is "message_handled" or "message_unhandled")
{
    ulong sum = 0, callbacks = 0;
    var client = workload == "message_handled" ? Client(e => e.HandleMessageCreated((_, ev) => { callbacks++; sum = unchecked(sum * 1099511628211 + ev.Message.Id + ev.Message.Author.Id); return Task.CompletedTask; })) : Client();
    await client.HandleDispatchAsync(Parse(guild));
    client.guilds[111111111111111111] = client.guilds[GuildId];
    Run = async () =>
    {
        sum = 1469598103934665603; callbacks = 0;
        for (int i = 0; i < operations; i++) await client.HandleDispatchAsync(Parse(message));
        if (workload == "message_handled" && callbacks != (ulong)operations) throw new InvalidDataException("callback count mismatch");
        return (sum, callbacks, "full_dispatch");
    };
}
else
{
    var state = Client();
    if (workload != "guild_create_state") await state.HandleDispatchAsync(Parse(guild));
    Run = async () =>
    {
        ulong sum = 1469598103934665603;
        if (workload == "guild_create_state")
        {
            for (int i = 0; i < operations; i++) await state.HandleDispatchAsync(Parse(guild));
            var cached = state.Guilds[GuildId];
            if (cached.Members.Count != 500 || cached.Roles.Count != 40 || cached.Channels.Count != 25) throw new InvalidDataException("cache mismatch");
            return (FirstUser ^ (FirstUser + 499), 0, "state_update");
        }
        var g = state.Guilds[GuildId];
        if (workload == "member_lookup")
        {
            ulong[] ids = [FirstUser, FirstUser + 250, FirstUser + 499, FirstUser + 9999];
            for (int i = 0; i < operations; i++) if (g.Members.TryGetValue(ids[i & 3], out var m)) sum = unchecked(sum * 1099511628211 + m.Id);
            return (sum, 0, "public_cache");
        }
        if (workload == "permission_resolve")
        {
            var channel = g.Channels[ChannelId]; var member = g.Members[FirstUser];
            for (int i = 0; i < operations; i++) { ulong p = ulong.Parse(channel.PermissionsFor(member).ToString()); if (p != 76800) throw new InvalidDataException($"permissions={p}"); sum = unchecked(sum * 1099511628211 + p); }
            return (sum, 0, "public_cache");
        }
        throw new NotSupportedException(workload);
    };
}

for (int i = 0; i < warmups; i++) _ = await Run();
for (int sample = 1; sample <= samples; sample++)
{
    var start = Stopwatch.GetTimestamp();
    var value = await Run();
    long elapsed = Stopwatch.GetElapsedTime(start).Ticks * 100;
    Write(new { schema = 1, library = Library, language = "C#", commit = Commit, runtime = System.Runtime.InteropServices.RuntimeInformation.FrameworkDescription, build = "Release", workload, coverage = value.coverage, sample, operations, elapsed_ns = elapsed, ns_per_op = (double)elapsed / operations, peak_rss_bytes = Process.GetCurrentProcess().PeakWorkingSet64, callbacks = value.callbacks, digest = Digest(value.sum) });
}
