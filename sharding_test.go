package starlings

import "testing"

func TestShardIDForGuild(t *testing.T) {
	c := testClient()
	c.makeShards(4, "wss://gateway.example")
	for want := 0; want < 4; want++ {
		guildID := Snowflake(uint64(want) << 22)
		if got := c.ShardIDForGuild(guildID); got != want {
			t.Fatalf("ShardIDForGuild(%d) = %d, want %d", guildID, got, want)
		}
	}
}

func TestHandlersAddedAfterShardingReachEveryShard(t *testing.T) {
	c := testClient()
	shards := c.makeShards(3, "wss://gateway.example")
	On(c, func(*MessageCreate) {})
	for i, shard := range shards {
		if shard.slotFor("MESSAGE_CREATE") == nil {
			t.Fatalf("shard %d did not receive handler", i)
		}
	}
}
