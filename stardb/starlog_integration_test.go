package stardb_test

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/razecrs/starlings"
	"github.com/razecrs/starlings/stardb"
)

func TestStarlogIntegration(t *testing.T) {
	logs := starlings.NewStarlog("database test",
		starlings.StarlogStreaming(),
		starlings.StarlogOutput(io.Discard),
		starlings.StarlogNoPets(),
	)
	defer logs.Close()
	store, err := stardb.OpenJSON(filepath.Join(t.TempDir(), "data.json"),
		stardb.WithLogger(logs.Logger()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := stardb.Save(context.Background(), store, "private-key", map[string]int{"count": 1}); err != nil {
		t.Fatal(err)
	}
	entries := logs.Snapshot()
	if len(entries) < 2 {
		t.Fatalf("expected open and put entries, got %d", len(entries))
	}
	last := entries[len(entries)-1]
	if last.Message != "stardb put" {
		t.Fatalf("unexpected Starlog message %q", last.Message)
	}
	for _, field := range last.Fields {
		if strings.Contains(field.Value, "private-key") {
			t.Fatal("database key leaked into Starlog")
		}
	}
}
