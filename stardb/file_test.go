package stardb

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type testValue struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Tags  []string `json:"tags"`
}

func TestFileStoresRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name string
		ext  string
		open func(string, ...Option) (*FileStore, error)
	}{
		{"json", ".json", OpenJSON},
		{"csv", ".csv", OpenCSV},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "store"+test.ext)
			store, err := test.open(path)
			if err != nil {
				t.Fatal(err)
			}
			want := testValue{Name: "commas, quotes \" and\nnewlines", Count: 7, Tags: []string{"a", "b"}}
			if err := store.Put(context.Background(), "guild:123", want); err != nil {
				t.Fatal(err)
			}
			if err := store.Put(context.Background(), "other", 42); err != nil {
				t.Fatal(err)
			}
			var got testValue
			if err := store.Get(context.Background(), "guild:123", &got); err != nil {
				t.Fatal(err)
			}
			if got.Name != want.Name || got.Count != want.Count || strings.Join(got.Tags, ",") != "a,b" {
				t.Fatalf("round trip mismatch: %#v", got)
			}
			keys, err := store.Keys(context.Background())
			if err != nil || strings.Join(keys, ",") != "guild:123,other" {
				t.Fatalf("keys=%v err=%v", keys, err)
			}
			if err := store.Delete(context.Background(), "other"); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(store.Delete(context.Background(), "other"), ErrNotFound) {
				t.Fatal("second delete should be ErrNotFound")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(store.Get(context.Background(), "guild:123", &got), ErrClosed) {
				t.Fatal("get after close should be ErrClosed")
			}
			reopened, err := test.open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if err := reopened.Get(context.Background(), "guild:123", &got); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUpdateIsAtomicAndTyped(t *testing.T) {
	store, err := OpenJSON(filepath.Join(t.TempDir(), "atomic.json"), WithBufferedFileWrites())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const workers = 32
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			_, err := Update(context.Background(), store, "count", 0, func(value *int) error {
				*value = *value + 1
				return nil
			})
			if err != nil {
				t.Errorf("Update: %v", err)
			}
		}()
	}
	group.Wait()
	got, err := Load[int](context.Background(), store, "count")
	if err != nil || got != workers {
		t.Fatalf("count=%d err=%v", got, err)
	}
}

func TestFileStoreEncryptionAndAssociatedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	key := bytes.Repeat([]byte{0x42}, 32)
	store, err := OpenJSON(path, WithEncryption(key))
	if err != nil {
		t.Fatal(err)
	}
	secret := "do-not-leak-this-value"
	if err := store.Put(context.Background(), "token", secret); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte(secret)) {
		t.Fatal("plaintext leaked to encrypted store")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJSON(path, WithEncryption(bytes.Repeat([]byte{0x42}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var got string
	if err := reopened.Get(context.Background(), "token", &got); err != nil || got != secret {
		t.Fatalf("got=%q err=%v", got, err)
	}
	wrong, err := OpenJSON(path, WithEncryption(bytes.Repeat([]byte{0x24}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	if err := wrong.Get(context.Background(), "token", &got); err == nil {
		t.Fatal("wrong encryption key unexpectedly succeeded")
	}
}

func TestFileStoreLimitsAndCorruption(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenJSON(filepath.Join(directory, "limited.json"), WithLimits(Limits{MaxKeyBytes: 4, MaxValueBytes: 8, MaxFileBytes: 256}))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if !errors.Is(store.Put(context.Background(), "abcde", 1), ErrInvalidKey) {
		t.Fatal("oversized key was accepted")
	}
	if !errors.Is(store.Put(context.Background(), "key", strings.Repeat("x", 20)), ErrTooLarge) {
		t.Fatal("oversized value was accepted")
	}
	corrupt := filepath.Join(directory, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte(`{"version":1,"records":{}} trailing`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJSON(corrupt); err == nil {
		t.Fatal("trailing corruption was accepted")
	}
}

func TestFileStoreRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks may require Windows developer mode")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	if err := os.WriteFile(target, []byte(`{"version":1,"records":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJSON(link); err == nil {
		t.Fatal("symlink database was accepted")
	}
}

func TestFileStoreConcurrentAccess(t *testing.T) {
	store, err := OpenJSON(filepath.Join(t.TempDir(), "race.json"), WithEncryption(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for i := 0; i < 50; i++ {
				if err := store.Put(context.Background(), "shared", testValue{Count: worker*100 + i}); err != nil {
					t.Error(err)
					return
				}
				var value testValue
				if err := store.Get(context.Background(), "shared", &value); err != nil {
					t.Error(err)
					return
				}
			}
		}(worker)
	}
	wait.Wait()
}

func TestLoggerRedactsKeyAndValue(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store, err := OpenJSON(filepath.Join(t.TempDir(), "log.json"), WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key, value := "private-user-id", "private-access-token"
	if err := store.Put(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	logs := output.String()
	if strings.Contains(logs, key) || strings.Contains(logs, value) {
		t.Fatalf("sensitive data appeared in logs: %s", logs)
	}
	if !strings.Contains(logs, keyHash(key)) {
		t.Fatal("redacted key hash missing from log")
	}
}

func TestInvalidEncryptionKey(t *testing.T) {
	_, err := OpenJSON(filepath.Join(t.TempDir(), "bad.json"), WithEncryption([]byte("short")))
	if err == nil {
		t.Fatal("short encryption key accepted")
	}
}

func TestBufferedFileWritesFlushAndClose(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "buffered.json")
	store, err := OpenJSON(path, WithBufferedFileWrites())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "first", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("buffered write reached disk before flush: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("flush did not persist store: %v", err)
	}
	if err := store.Put(ctx, "second", 2); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var got int
	if err := reopened.Get(ctx, "second", &got); err != nil || got != 2 {
		t.Fatalf("close did not flush buffered write: got=%d err=%v", got, err)
	}
	if !errors.Is(store.Flush(), ErrClosed) {
		t.Fatal("flush after close should be ErrClosed")
	}
}

func TestBufferedCloseFailureLeavesStoreOpen(t *testing.T) {
	ctx := context.Background()
	store, err := OpenJSON(
		filepath.Join(t.TempDir(), "too-small.json"),
		WithBufferedFileWrites(),
		WithLimits(Limits{MaxFileBytes: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "key", "value"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(store.Close(), ErrTooLarge) {
		t.Fatal("close should report the failed buffered flush")
	}
	var got string
	if err := store.Get(ctx, "key", &got); err != nil || got != "value" {
		t.Fatalf("failed close must leave the store retryable: got=%q err=%v", got, err)
	}
}

func BenchmarkFileStorePut(b *testing.B) {
	for _, test := range []struct {
		name    string
		open    func(string, ...Option) (*FileStore, error)
		options []Option
	}{
		{name: "json/durable", open: OpenJSON},
		{name: "json/buffered", open: OpenJSON, options: []Option{WithBufferedFileWrites()}},
		{name: "csv/durable", open: OpenCSV},
		{name: "csv/buffered", open: OpenCSV, options: []Option{WithBufferedFileWrites()}},
	} {
		b.Run(test.name, func(b *testing.B) {
			store, err := test.open(filepath.Join(b.TempDir(), "store"), test.options...)
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			ctx := context.Background()
			value := testValue{Name: "benchmark", Count: 42, Tags: []string{"fast", "safe"}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := store.Put(ctx, "same-key", value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
