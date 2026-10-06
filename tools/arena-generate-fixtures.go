// Command arena-generate-fixtures creates the immutable payloads consumed by
// every language adapter. Keep generation deterministic: fixture hashes are
// part of the arena evidence.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	guildID   = "41771983423143937"
	ownerID   = "80351110224678912"
	roleID    = "900000000000000001"
	channelID = "800000000000000000"
)

func main() {
	directory := filepath.FromSlash("benchmarks/arena/fixtures")
	must(os.MkdirAll(directory, 0o755))
	files := map[string][]byte{
		"message-create.json":   marshal(messageCreate()),
		"guild-create-500.json": marshal(guildCreate()),
		"permission-case.json":  marshal(permissionCase()),
		"malformed-frame.json":  []byte(`{"op":0,"t":"MESSAGE_CREATE","s":8,"d":{"id":`),
	}
	manifest := make(map[string]string, len(files))
	for name, contents := range files {
		contents = append(contents, '\n')
		must(os.WriteFile(filepath.Join(directory, name), contents, 0o644))
		sum := sha256.Sum256(contents)
		manifest[name] = "sha256:" + hex.EncodeToString(sum[:])
	}
	must(os.WriteFile(filepath.Join(directory, "manifest.json"), append(marshal(manifest), '\n'), 0o644))
}

func messageCreate() any {
	return map[string]any{
		"op": 0, "s": 7, "t": "MESSAGE_CREATE",
		"d": map[string]any{
			"id": "1234567890123456789", "channel_id": "987654321098765432",
			"guild_id": "111111111111111111", "content": "hello there",
			"timestamp": "2026-01-02T03:04:05.000000+00:00", "type": 0,
			"mention_everyone": false, "mentions": []any{}, "mention_roles": []any{},
			"attachments": []any{}, "embeds": []any{}, "pinned": false,
			"flags": 0, "components": []any{},
			"author": map[string]any{
				"id": "222222222222222222", "username": "tester",
				"global_name": "Starlings Tester", "discriminator": "0", "avatar": nil, "bot": false,
			},
		},
	}
}

func guildCreate() any {
	roles := make([]any, 40)
	roles[0] = map[string]any{
		"id": guildID, "name": "@everyone", "color": 0, "position": 0,
		"permissions": "68608", "hoist": false, "managed": false, "mentionable": false,
	}
	roles[1] = map[string]any{
		"id": roleID, "name": "arena-role", "color": 5793266, "position": 1,
		"permissions": "1056768", "hoist": true, "managed": false, "mentionable": true,
	}
	for i := 2; i < len(roles); i++ {
		roles[i] = map[string]any{
			"id": fmt.Sprintf("%d", 900000000000000000+i), "name": fmt.Sprintf("role-%d", i),
			"color": i * 1000, "position": i, "permissions": "1024",
			"hoist": false, "managed": false, "mentionable": false,
		}
	}
	channels := make([]any, 25)
	for i := range channels {
		channel := map[string]any{
			"id": fmt.Sprintf("%d", 800000000000000000+i), "type": 0,
			"name": fmt.Sprintf("channel-%d", i), "position": i,
			"topic": "a topic that is reasonably long, as topics tend to be",
			"nsfw":  false, "rate_limit_per_user": 0,
		}
		if i == 0 {
			channel["permission_overwrites"] = []any{
				map[string]any{"id": guildID, "type": 0, "allow": "0", "deny": "2048"},
				map[string]any{"id": roleID, "type": 0, "allow": "2048", "deny": "8192"},
				map[string]any{"id": "700000000000000000", "type": 1, "allow": "8192", "deny": "1048576"},
			}
		} else {
			channel["permission_overwrites"] = []any{}
		}
		channels[i] = channel
	}
	members := make([]any, 500)
	for i := range members {
		rolesForMember := []any{}
		if i == 0 {
			rolesForMember = []any{roleID}
		}
		members[i] = map[string]any{
			"user": map[string]any{
				"id":       fmt.Sprintf("%d", 700000000000000000+i),
				"username": fmt.Sprintf("user%d", i), "global_name": fmt.Sprintf("User %d", i),
				"discriminator": "0", "avatar": "a_1234567890abcdef1234567890abcdef", "bot": false,
			},
			"nick": nil, "roles": rolesForMember,
			"joined_at": "2021-05-19T00:00:00.000000+00:00",
			"deaf":      false, "mute": false, "pending": false, "flags": 0,
		}
	}
	return map[string]any{
		"op": 0, "s": 3, "t": "GUILD_CREATE",
		"d": map[string]any{
			"id": guildID, "name": "Benchmark Guild", "owner_id": ownerID,
			"region":       "us-central",
			"member_count": 500, "large": true, "unavailable": false,
			"roles": roles, "channels": channels, "members": members,
			"threads": []any{}, "emojis": []any{}, "stickers": []any{},
			"features": []any{}, "voice_states": []any{}, "presences": []any{},
			"afk_timeout": 300, "verification_level": 0, "default_message_notifications": 0,
			"explicit_content_filter": 0, "mfa_level": 0, "system_channel_flags": 0,
			"premium_tier": 0, "preferred_locale": "en-US",
		},
	}
}

func permissionCase() any {
	return map[string]any{
		"guild_id": guildID, "channel_id": channelID, "user_id": "700000000000000000",
		"everyone_permissions": "68608", "role_permissions": "1056768",
		"everyone_deny": "2048", "role_allow": "2048", "role_deny": "8192",
		"member_allow": "8192", "member_deny": "1048576",
		"expected_permissions": "76800",
	}
}

func marshal(value any) []byte {
	encoded, err := json.MarshalIndent(value, "", "  ")
	must(err)
	return encoded
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
