package starlings

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// atlasSizedGuildCreate builds a GUILD_CREATE payload shaped like the guilds
// in the Atlas benchmark: real-looking IDs, hashes and names, so string and
// map sizes resemble live traffic rather than empty structs.
func atlasSizedGuildCreate(guild, members, channels, roles int) []byte {
	id := func(kind, n int) string { return fmt.Sprintf("%d", 1100000000000000000+guild*10000000+kind*100000+n) }
	var b strings.Builder
	fmt.Fprintf(&b, `{"id":%q,"name":"Guild %d","owner_id":%q,"member_count":%d,"roles":[`, id(0, 0), guild, id(1, 0), members)
	for r := range roles {
		if r > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%q,"name":"Role %d","color":%d,"position":%d,"permissions":"1071698660929"}`, id(2, r), r, r*1000, r)
	}
	b.WriteString(`],"channels":[`)
	for c := range channels {
		if c > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%q,"type":0,"name":"channel-%d","position":%d,"topic":"Topic for channel %d","permission_overwrites":[{"id":%q,"type":0,"allow":"1024","deny":"0"}]}`, id(3, c), c, c, c, id(2, 0))
	}
	b.WriteString(`],"members":[`)
	for m := range members {
		if m > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"user":{"id":%q,"username":"member_%d","global_name":"Member %d","avatar":"a1b2c3d4e5f60718293a4b5c6d7e8f90"},"roles":[%q,%q,%q],"joined_at":"2025-03-01T12:00:00.000000+00:00","deaf":false,"mute":false}`,
			id(1, m), m, m, id(2, m%roles), id(2, (m+1)%roles), id(2, (m+2)%roles))
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse
}

// TestStateMemoryReport prints the retained heap of the default cache after
// Atlas-sized traffic: 8 guilds and 2,000 members. It is a measurement, not
// an assertion, so it only runs when STARLINGS_MEMORY_REPORT is set.
func TestStateMemoryReport(t *testing.T) {
	if os.Getenv("STARLINGS_MEMORY_REPORT") == "" {
		t.Skip("set STARLINGS_MEMORY_REPORT=1 to print cache memory use")
	}
	const guilds, members, channels, roles = 8, 250, 40, 30
	payloads := make([][]byte, guilds)
	for g := range guilds {
		payloads[g] = atlasSizedGuildCreate(g, members, channels, roles)
	}

	for _, tc := range []struct {
		name   string
		config StateConfig
	}{
		{"default", DefaultStateConfig()},
		{"minimal", MinimalStateConfig()},
	} {
		base := heapInUse()
		state := newStateWithConfig(tc.config)
		for _, payload := range payloads {
			var e GuildCreate
			if err := json.Unmarshal(payload, &e); err != nil {
				t.Fatal(err)
			}
			if err := state.Apply(&e); err != nil {
				t.Fatal(err)
			}
		}
		retained := heapInUse() - base
		t.Logf("%-8s retained %7.2f MB for %d members (%d bytes/member)",
			tc.name, float64(retained)/(1<<20), guilds*members, retained/uint64(guilds*members))
		runtime.KeepAlive(state)
	}
}

// TestStarPetMemoryReport prints the heap kept by three pictured pets after
// each has rendered a frame.
func TestStarPetMemoryReport(t *testing.T) {
	if os.Getenv("STARLINGS_MEMORY_REPORT") == "" {
		t.Skip("set STARLINGS_MEMORY_REPORT=1 to print pet memory use")
	}
	useBundledPetArt(t)
	base := heapInUse()
	pets := []StarlogPet{NewStarPet(StarPetNova), NewStarPet(StarPetComet), NewStarPet(StarPetNebula)}
	for _, pet := range pets {
		if _, ok := renderStarPetPicture(pet, StarlogBuild, time.Unix(0, 0), 22); !ok {
			t.Fatalf("pet %q did not render", pet.Name)
		}
	}
	t.Logf("three pictured pets retain %.2f MB", float64(heapInUse()-base)/(1<<20))
	runtime.KeepAlive(pets)
}
