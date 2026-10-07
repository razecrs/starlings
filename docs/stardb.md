# StarDB

StarDB is optional persistence for bots that do not want to assemble a storage
layer before saving their first setting. Importing `github.com/razecrs/starlings` does not
open a database; persistence lives in the separate
`github.com/razecrs/starlings/stardb` package.

Every backend implements the same small interface:

```go
type Store interface {
    Get(context.Context, string, any) error
    Put(context.Context, string, any) error
    Delete(context.Context, string) error
    Keys(context.Context) ([]string, error)
    Close() error
}
```

Values are ordinary JSON-compatible Go values. `Load[T]` and `Save` remove the
usual temporary-variable boilerplate.

## Local JSON or CSV

```go
db, err := stardb.OpenJSON("data/settings.json")
if err != nil {
    log.Fatal(err)
}
defer db.Close()

ctx := context.Background()
err = stardb.Save(ctx, db, "guild:123", GuildSettings{Prefix: "!"})
settings, err := stardb.Load[GuildSettings](ctx, db, "guild:123")
```

Use `Update` for read-modify-write operations so concurrent commands cannot
silently overwrite each other:

```go
settings, err = stardb.Update(ctx, db, "guild:123", GuildSettings{},
    func(value *GuildSettings) error {
        value.CommandsRun++
        return nil
    })
```

Local files serialize the operation inside the process. SQL uses a conditional
compare-and-swap, and Firebase uses ETags with bounded conflict retries. The
callback can run more than once on a contended remote value, so it must only
change the supplied value and must not send messages or perform other external
side effects. Backends that cannot promise atomicity return `ErrNotAtomic`.

Change only the constructor for CSV:

```go
db, err := stardb.OpenCSV("data/settings.csv")
```

JSON and CSV use atomic, write-through persistence by default. For rebuildable
state or write-heavy workloads, buffer changes in memory and choose when to pay
the disk sync cost:

```go
db, err := stardb.OpenJSON("data/cache.json", stardb.WithBufferedFileWrites())
if err != nil {
    return err
}
defer db.Close() // flushes pending writes

// Flush at an application-defined checkpoint when needed.
err = db.Flush()
```

A crash can lose changes made after the most recent `Flush`. `Close` flushes,
and returns the persistence error without closing the store so the caller can
retry. Leave buffering off for data that must survive immediately after each
successful `Put` or `Delete`.

Both stores write owner-only temporary files, sync them, and atomically replace
the prior database. Existing symlink database paths are rejected. Reads and
writes are safe across goroutines in one process. A local file must not be
opened by multiple processes; use SQL or a hosted backend for that.

## Encryption at rest

```go
key := []byte(os.Getenv("STARDB_KEY")) // exactly 32 bytes
db, err := stardb.OpenJSON("data/private.json", stardb.WithEncryption(key))
```

Encryption uses AES-256-GCM with a new cryptographic nonce for every write and
binds ciphertext to its record key. Record keys themselves remain visible.
StarDB copies the encryption key and wipes its copy on close. Keep the original
in a secret manager or protected environment variable, never source control.
Back up the key separately: losing it makes the data unrecoverable. Key
rotation is deliberately not automatic.

## Starlog integration

```go
logs := starlings.NewStarlog("my bot", starlings.StarlogStreaming())
defer logs.Close()

db, err := stardb.OpenJSON("data/settings.json",
    stardb.WithLogger(logs.Logger()),
)
```

The same option accepts any standard `*slog.Logger`; logging is optional.
StarDB logs backend and operation information, but replaces keys with short
SHA-256 fingerprints and never logs values, credentials, response bodies, or
remote URLs.

## SQL

StarDB uses `database/sql`, so the application chooses its audited driver:

```go
sqlDB, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
db, err := stardb.OpenSQL(sqlDB, "star_data", stardb.SQLPostgres)
if err == nil {
    err = db.EnsureSchema(ctx)
}
```

`SQLSQLite`, `SQLPostgres`, and `SQLMySQL` are supported. Table identifiers are
strictly validated and every key/value is a query parameter. StarDB does not
own the supplied `*sql.DB` unless `WithOwnedSQLConnection()` is passed. Set
driver pool sizes, connection lifetime, TLS, and per-operation context
deadlines for the deployment.

## Firebase Realtime Database

```go
db, err := stardb.OpenFirebase(stardb.FirebaseConfig{
    DatabaseURL: os.Getenv("FIREBASE_DATABASE_URL"),
    Token:       os.Getenv("FIREBASE_TOKEN"),
    Prefix:      "my_bot",
})
```

Hosted URLs must use HTTPS. The token is sent only as a Bearer header, redirects
are refused, responses are bounded, and deletes use ETags to avoid deleting a
concurrently replaced record. Configure Firebase Security Rules to deny all
other paths and grant this identity only the required prefix. Never depend on
client-side code to enforce access.

Firebase forbids `.`, `#`, `$`, `[`, `]`, and `/` in StarDB keys and prefixes.

## Supabase

Create a table with a unique text key and a `jsonb` value, then:

```go
db, err := stardb.OpenSupabase(stardb.SupabaseConfig{
    ProjectURL: os.Getenv("SUPABASE_URL"),
    APIKey:     os.Getenv("SUPABASE_KEY"),
    Table:      "star_data",
})
```

The default columns are `key` and `value`; `KeyColumn` and `ValueColumn` can
change them. Identifiers are strictly validated, credentials are headers only,
redirects are refused, and upserts use the unique key.
Supabase record keys accept letters, digits, `-`, `_`, `.`, `~`, and `:`; this
deliberately excludes PostgREST filter syntax.

Enable Row Level Security and create the smallest policies the bot needs. An
anon key is not authorization by itself. A `service_role` key bypasses RLS and
must never appear in a distributed client, log, repository, Discord message,
or browser bundle. Prefer a restricted server-side identity.

## Limits and timeouts

Defaults bound keys to 512 bytes, values to 4 MiB, local files to 64 MiB, and
remote responses to 8 MiB. Tighten them for known workloads:

```go
stardb.WithLimits(stardb.Limits{
    MaxKeyBytes:      128,
    MaxValueBytes:    64 << 10,
    MaxFileBytes:     8 << 20,
    MaxResponseBytes: 256 << 10,
})
```

Remote stores use a 15-second client timeout by default. Production calls
should also carry shorter context deadlines. Ordinary writes are not retried;
only `Update` retries a detected compare-and-swap conflict. A `RemoteError`
reports whether a failure is temporary without leaking the server response.
