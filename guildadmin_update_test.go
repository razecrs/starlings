package starlings

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestScheduledEventUpdateIsPartial(t *testing.T) {
	body, err := json.Marshal(ScheduledEventUpdate{Status: EventCancelled})
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, `"status":4`) {
		t.Fatalf("update = %s, want status", body)
	}
	for _, absent := range []string{"name", "scheduled_start_time", "privacy_level", "entity_type"} {
		if strings.Contains(got, `"`+absent+`"`) {
			t.Fatalf("partial update = %s, unexpectedly contains %s", body, absent)
		}
	}
}

func TestScheduledEventUpdateCanClearNullableFields(t *testing.T) {
	body, err := json.Marshal(ScheduledEventUpdate{
		ClearChannel: true, ClearScheduledEnd: true, ClearEntityMetadata: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, field := range []string{"channel_id", "scheduled_end_time", "entity_metadata"} {
		if !strings.Contains(got, `"`+field+`":null`) {
			t.Fatalf("clear update = %s, missing null %s", body, field)
		}
	}
}
