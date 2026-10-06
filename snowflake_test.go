package starlings

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSnowflakeTime(t *testing.T) {
	// A known ID from Discord's own documentation.
	const id Snowflake = 175928847299117063
	want := time.Date(2016, time.April, 30, 11, 18, 25, 796_000_000, time.UTC)
	if got := id.Time(); !got.Equal(want) {
		t.Errorf("Time() = %v, want %v", got, want)
	}
}

func TestSnowflakeJSONRoundTrip(t *testing.T) {
	type payload struct {
		ID     Snowflake `json:"id"`
		Absent Snowflake `json:"absent"`
	}

	var p payload
	if err := json.Unmarshal([]byte(`{"id":"175928847299117063","absent":null}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != 175928847299117063 {
		t.Errorf("ID = %d, want 175928847299117063", p.ID)
	}
	if !p.Absent.IsZero() {
		t.Errorf("Absent = %d, want zero", p.Absent)
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"id":"175928847299117063","absent":null}` {
		t.Errorf("Marshal = %s", b)
	}
}

func TestSnowflakeUnmarshalNumber(t *testing.T) {
	var s Snowflake
	if err := json.Unmarshal([]byte(`175928847299117063`), &s); err != nil {
		t.Fatal(err)
	}
	if s != 175928847299117063 {
		t.Errorf("got %d", s)
	}
}
