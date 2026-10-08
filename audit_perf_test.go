package starlings

import (
	"encoding/json/v2"
	"reflect"
	"testing"
)

func BenchmarkGuildSnapshot(b *testing.B) {
	s, id := snapshotBenchmarkState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		g, ok := s.Guild(id)
		if !ok || len(g.Members) != 2000 {
			b.Fatal("incomplete snapshot")
		}
	}
}

func snapshotBenchmarkState(b *testing.B) (*State, Snowflake) {
	b.Helper()
	var event GuildCreate
	if err := json.Unmarshal(atlasSizedGuildCreate(1, 2000, 40, 30), &event); err != nil {
		b.Fatal(err)
	}
	s := newState()
	if err := s.Apply(&event); err != nil {
		b.Fatal(err)
	}
	return s, event.ID
}

func BenchmarkGuildInfo(b *testing.B) {
	s, id := snapshotBenchmarkState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		g, ok := s.GuildInfo(id)
		if !ok || g.MemberCount != 2000 {
			b.Fatal("incorrect metadata")
		}
	}
}

func BenchmarkMemberCount(b *testing.B) {
	s, id := snapshotBenchmarkState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if s.MemberCount(id) != 2000 {
			b.Fatal("incorrect count")
		}
	}
}

func BenchmarkTypedHandlerEmpty(b *testing.B) {
	h := adaptHandler("/test", func() string { return "" }, argsFromOptions, "test")
	i := &InteractionCreate{}
	b.ReportAllocs()
	for b.Loop() {
		if err := h.call(i); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRouteParameter(b *testing.B) {
	var value int64
	v := reflect.ValueOf(&value).Elem()
	b.ReportAllocs()
	for b.Loop() {
		if err := setText(v, reflect.Int64, "1548760732286451903"); err != nil {
			b.Fatal(err)
		}
	}
}
