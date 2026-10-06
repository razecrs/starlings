package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	guildID   = "41771983423143937"
	channelID = "800000000000000000"
	firstUser = uint64(700000000000000000)
)

type result struct {
	Schema       int     `json:"schema"`
	Library      string  `json:"library"`
	Language     string  `json:"language"`
	Commit       string  `json:"commit"`
	Runtime      string  `json:"runtime"`
	Build        string  `json:"build"`
	Workload     string  `json:"workload"`
	Coverage     string  `json:"coverage"`
	Sample       int     `json:"sample"`
	Operations   int     `json:"operations"`
	ElapsedNS    int64   `json:"elapsed_ns"`
	NSPerOp      float64 `json:"ns_per_op"`
	PeakRSSBytes int64   `json:"peak_rss_bytes"`
	Callbacks    uint64  `json:"callbacks"`
	Digest       string  `json:"digest"`
}

type adapter struct{ fixtures map[string][]byte }

func main() {
	if len(os.Args) < 2 {
		fatal("usage: adapter <info|verify|cold-start|bench>")
	}
	a, err := loadAdapter()
	if err != nil {
		fatal(err.Error())
	}
	switch os.Args[1] {
	case "info":
		write(map[string]any{
			"schema": 1, "library": "go_discordgo", "language": "Go", "commit": commit(),
			"runtime": runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH, "build": "release",
			"supported": []string{"message_handled", "message_unhandled", "guild_create_state", "member_lookup", "permission_resolve", "malformed_frame"},
			"notes":     "public typed models plus the same exported State.OnInterface consumer used internally; user callback invocation is adapter-owned because DiscordGo has no public offline frame dispatcher",
		})
	case "verify":
		if err := a.verify(); err != nil {
			fatal(err.Error())
		}
		write(map[string]any{"schema": 1, "library": "go_discordgo", "verified": true})
	case "cold-start":
		_, _ = discordgo.New("Bot offline")
		write(baseResult("cold_start", "library_load", 0, 1, 0, 0, digest("ready")))
	case "bench":
		if len(os.Args) != 6 {
			fatal("usage: adapter bench <workload> <operations> <warmup-batches> <measured-batches>")
		}
		if err := a.bench(os.Args[2], positive(os.Args[3]), nonnegative(os.Args[4]), positive(os.Args[5])); err != nil {
			fatal(err.Error())
		}
	default:
		fatal("unknown adapter command")
	}
}

func loadAdapter() (*adapter, error) {
	directory := os.Getenv("ARENA_FIXTURES")
	if directory == "" {
		directory = filepath.FromSlash("benchmarks/arena/fixtures")
	}
	fixtures := make(map[string][]byte)
	for _, name := range []string{"message-create.json", "guild-create-500.json", "malformed-frame.json"} {
		contents, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return nil, err
		}
		fixtures[name] = contents
	}
	return &adapter{fixtures: fixtures}, nil
}

func (a *adapter) verify() error {
	session := newSession()
	message, err := decodeMessage(a.fixtures["message-create.json"])
	if err != nil {
		return err
	}
	if err := session.State.OnInterface(session, message); err != nil {
		return err
	}
	if message.ID != "1234567890123456789" || message.ChannelID != "987654321098765432" ||
		message.GuildID != "111111111111111111" || message.Author == nil || message.Author.ID != "222222222222222222" || message.Content != "hello there" {
		return errors.New("message did not match canonical fixture")
	}
	guild, err := decodeGuild(a.fixtures["guild-create-500.json"])
	if err != nil {
		return err
	}
	setGuildIDs(guild.Guild)
	if err := session.State.OnInterface(session, guild); err != nil {
		return err
	}
	cached, err := session.State.Guild(guildID)
	if err != nil || cached.OwnerID != "80351110224678912" || len(cached.Members) != 500 || len(cached.Roles) != 40 || len(cached.Channels) != 25 {
		return fmt.Errorf("state mismatch: guild=%v err=%v", cached != nil, err)
	}
	for _, offset := range []uint64{0, 250, 499} {
		id := strconv.FormatUint(firstUser+offset, 10)
		member, err := session.State.Member(guildID, id)
		if err != nil || member.User == nil || member.User.ID != id {
			return fmt.Errorf("member %s missing", id)
		}
	}
	permissions, err := session.State.UserChannelPermissions(strconv.FormatUint(firstUser, 10), channelID)
	if err != nil || permissions != 76800 {
		return fmt.Errorf("permissions=%d err=%v", permissions, err)
	}
	if _, err := decodeMessage(a.fixtures["malformed-frame.json"]); err == nil {
		return errors.New("malformed frame was accepted")
	}
	return nil
}

func (a *adapter) bench(workload string, operations, warmup, samples int) error {
	run, coverage, callbacksExpected, err := a.workload(workload)
	if err != nil {
		return err
	}
	for range warmup {
		if _, _, err := run(operations); err != nil {
			return err
		}
	}
	for sample := 1; sample <= samples; sample++ {
		started := time.Now()
		checksum, callbacks, err := run(operations)
		elapsed := time.Since(started)
		if err != nil {
			return err
		}
		if callbacksExpected && callbacks != uint64(operations) {
			return fmt.Errorf("callback count=%d want=%d", callbacks, operations)
		}
		row := baseResult(workload, coverage, sample, operations, elapsed.Nanoseconds(), callbacks, digest(strconv.FormatUint(checksum, 10)))
		row.NSPerOp = float64(row.ElapsedNS) / float64(operations)
		write(row)
	}
	return nil
}

func (a *adapter) workload(name string) (func(int) (uint64, uint64, error), string, bool, error) {
	session := newSession()
	switch name {
	case "message_handled":
		return func(operations int) (uint64, uint64, error) {
			checksum := uint64(1469598103934665603)
			var callbacks uint64
			for range operations {
				message, err := decodeMessage(a.fixtures["message-create.json"])
				if err != nil {
					return 0, 0, err
				}
				if err := session.State.OnInterface(session, message); err != nil {
					return 0, 0, err
				}
				callbacks++ // DiscordGo's callback entry point is not public.
				checksum = checksum*1099511628211 + parseID(message.ID) + parseID(message.Author.ID)
			}
			return checksum, callbacks, nil
		}, "state_update", true, nil
	case "message_unhandled":
		return func(operations int) (uint64, uint64, error) {
			for range operations {
				message, err := decodeMessage(a.fixtures["message-create.json"])
				if err != nil {
					return 0, 0, err
				}
				if err := session.State.OnInterface(session, message); err != nil {
					return 0, 0, err
				}
			}
			message, err := session.State.Message("987654321098765432", "1234567890123456789")
			if err != nil {
				return 0, 0, err
			}
			return parseID(message.ID), 0, nil
		}, "state_update", false, nil
	case "guild_create_state":
		return func(operations int) (uint64, uint64, error) {
			for range operations {
				guild, err := decodeGuild(a.fixtures["guild-create-500.json"])
				if err != nil {
					return 0, 0, err
				}
				setGuildIDs(guild.Guild)
				if err := session.State.OnInterface(session, guild); err != nil {
					return 0, 0, err
				}
			}
			guild, err := session.State.Guild(guildID)
			if err != nil || len(guild.Members) != 500 {
				return 0, 0, fmt.Errorf("guild state: %w", err)
			}
			return parseID(guild.Members[0].User.ID) + parseID(guild.Members[499].User.ID), 0, nil
		}, "state_update", false, nil
	case "member_lookup":
		guild, err := decodeGuild(a.fixtures["guild-create-500.json"])
		if err != nil {
			return nil, "", false, err
		}
		setGuildIDs(guild.Guild)
		if err := session.State.OnInterface(session, guild); err != nil {
			return nil, "", false, err
		}
		ids := [...]uint64{firstUser, firstUser + 250, firstUser + 499, firstUser + 9999}
		return func(operations int) (uint64, uint64, error) {
			checksum := uint64(1469598103934665603)
			for i := range operations {
				member, err := session.State.Member(guildID, strconv.FormatUint(ids[i&3], 10))
				if err == nil {
					checksum = checksum*1099511628211 + parseID(member.User.ID)
				}
			}
			return checksum, 0, nil
		}, "public_cache", false, nil
	case "permission_resolve":
		guild, err := decodeGuild(a.fixtures["guild-create-500.json"])
		if err != nil {
			return nil, "", false, err
		}
		setGuildIDs(guild.Guild)
		if err := session.State.OnInterface(session, guild); err != nil {
			return nil, "", false, err
		}
		return func(operations int) (uint64, uint64, error) {
			checksum := uint64(1469598103934665603)
			for range operations {
				permissions, err := session.State.UserChannelPermissions(strconv.FormatUint(firstUser, 10), channelID)
				if err != nil || permissions != 76800 {
					return 0, 0, fmt.Errorf("permissions=%d err=%v", permissions, err)
				}
				checksum = checksum*1099511628211 + uint64(permissions)
			}
			return checksum, 0, nil
		}, "public_cache", false, nil
	default:
		return nil, "", false, fmt.Errorf("unsupported workload %q", name)
	}
}

func decodeMessage(frame []byte) (*discordgo.MessageCreate, error) {
	var envelope discordgo.Event
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return nil, err
	}
	if envelope.Operation != 0 || envelope.Type != "MESSAGE_CREATE" {
		return nil, errors.New("not a MESSAGE_CREATE dispatch")
	}
	var message discordgo.MessageCreate
	if err := json.Unmarshal(envelope.RawData, &message); err != nil {
		return nil, err
	}
	return &message, nil
}

func decodeGuild(frame []byte) (*discordgo.GuildCreate, error) {
	var envelope discordgo.Event
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return nil, err
	}
	if envelope.Operation != 0 || envelope.Type != "GUILD_CREATE" {
		return nil, errors.New("not a GUILD_CREATE dispatch")
	}
	var guild discordgo.GuildCreate
	if err := json.Unmarshal(envelope.RawData, &guild); err != nil {
		return nil, err
	}
	return &guild, nil
}

func setGuildIDs(guild *discordgo.Guild) {
	for _, channel := range guild.Channels {
		channel.GuildID = guild.ID
	}
	for _, member := range guild.Members {
		member.GuildID = guild.ID
	}
	for _, voice := range guild.VoiceStates {
		voice.GuildID = guild.ID
	}
}

func newSession() *discordgo.Session {
	session, err := discordgo.New("Bot offline")
	if err != nil {
		panic(err)
	}
	session.SyncEvents = true
	session.StateEnabled = true
	session.State.MaxMessageCount = 100
	_ = session.State.GuildAdd(&discordgo.Guild{
		ID: "111111111111111111",
		Channels: []*discordgo.Channel{{
			ID: "987654321098765432", GuildID: "111111111111111111", Type: discordgo.ChannelTypeGuildText,
		}},
	})
	return session
}

func parseID(value string) uint64 {
	parsed, _ := strconv.ParseUint(value, 10, 64)
	return parsed
}

func baseResult(workload, coverage string, sample, operations int, elapsed int64, callbacks uint64, checksum string) result {
	return result{Schema: 1, Library: "go_discordgo", Language: "Go", Commit: commit(), Runtime: runtime.Version(), Build: "release", Workload: workload, Coverage: coverage, Sample: sample, Operations: operations, ElapsedNS: elapsed, Callbacks: callbacks, Digest: checksum}
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func commit() string {
	if value := os.Getenv("ARENA_COMMIT"); value != "" {
		return value
	}
	return "f43dd94faaacd5b163e9e783f14b5bd8be639fc9"
}

func positive(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		fatal("expected positive integer")
	}
	return parsed
}

func nonnegative(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		fatal("expected non-negative integer")
	}
	return parsed
}

func write(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
func fatal(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
