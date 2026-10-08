package starlings

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPurgeSplitsRecentAndOldMessages(t *testing.T) {
	now := time.Now()
	idAt := func(at time.Time, n int) Snowflake {
		return Snowflake(uint64(at.UnixMilli()-discordEpoch)<<22 | uint64(n))
	}
	var page []string
	for n := range 5 {
		page = append(page, fmt.Sprintf(`{"id":"%d","author":{"id":"9"}}`, idAt(now.Add(-time.Hour), 10-n)))
	}
	for n := range 3 {
		page = append(page, fmt.Sprintf(`{"id":"%d","author":{"id":"8"}}`, idAt(now.Add(-20*24*time.Hour), 3-n)))
	}
	log := &callLog{}
	c := clientServedBy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprintf(w, "[%s]", strings.Join(page, ","))
			return
		}
		log.ServeHTTP(w, r)
	}))
	res, err := c.Purge(context.Background(), 42, PurgeOptions{Count: 8, MaxOld: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 8 || res.Deleted != 7 || res.OldDeleted != 2 || res.OldSkipped != 1 {
		t.Fatalf("result = %+v", res)
	}
	calls := log.get()
	if len(calls) != 3 || calls[0] != "POST /channels/42/messages/bulk-delete" {
		t.Fatalf("calls = %v", calls)
	}

	log.calls = nil
	res, err = c.Purge(context.Background(), 42, PurgeOptions{Count: 10, Match: func(m *Message) bool { return m.Author.ID == 8 }, MaxOld: -1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 3 || res.Deleted != 0 || res.OldSkipped != 3 {
		t.Fatalf("filtered result = %+v", res)
	}
}
