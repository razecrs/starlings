package stardb

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzOpenJSONRejectsCorruptionWithoutPanicking(f *testing.F) {
	f.Add([]byte(`{"version":1,"records":{}}`))
	f.Add([]byte(`{"version":2,"records":null}`))
	f.Add([]byte(`{"version":1,"records":{"key":{"v":1,"json":{"x":1}}}}`))
	f.Add([]byte{0, 1, 2, 3})
	f.Fuzz(func(t *testing.T, contents []byte) {
		if len(contents) > 1<<20 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "fuzz.json")
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		store, err := OpenJSON(path, WithLimits(Limits{MaxFileBytes: 1 << 20, MaxValueBytes: 1 << 18}))
		if err == nil {
			_ = store.Close()
		}
	})
}

func FuzzOpenCSVRejectsCorruptionWithoutPanicking(f *testing.F) {
	f.Add([]byte("key,version,json,ciphertext,nonce\nexample,1,123,,\n"))
	f.Add([]byte("wrong,header\n"))
	f.Add([]byte{0xff, 0xfe, 0xfd})
	f.Fuzz(func(t *testing.T, contents []byte) {
		if len(contents) > 1<<20 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "fuzz.csv")
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		store, err := OpenCSV(path, WithLimits(Limits{MaxFileBytes: 1 << 20, MaxValueBytes: 1 << 18}))
		if err == nil {
			_ = store.Close()
		}
	})
}
