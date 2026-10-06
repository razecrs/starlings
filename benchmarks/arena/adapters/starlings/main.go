package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/razecrs/starlings"
)

const (
	guildID   = starlings.Snowflake(41771983423143937)
	channelID = starlings.Snowflake(800000000000000000)
	firstUser = starlings.Snowflake(700000000000000000)
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

type adapter struct {
	fixtures map[string][]byte
}

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
			"schema": 1, "library": "starlings", "language": "Go",
			"commit": commit(), "runtime": runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH,
			"build": "release", "supported": []string{
				"message_handled", "message_unhandled", "guild_create_state",
				"member_lookup", "permission_resolve", "malformed_frame",
			},
		})
	case "verify":
		if err := a.verify(); err != nil {
			fatal(err.Error())
		}
		write(map[string]any{"schema": 1, "library": "starlings", "verified": true})
	case "cold-start":
		_ = newClient()
		write(result{Schema: 1, Library: "starlings", Language: "Go", Commit: commit(), Runtime: runtime.Version(), Build: "release", Workload: "cold_start", Coverage: "library_load", Operations: 1, Digest: digest("ready")})
	case "bench":
		if len(os.Args) != 6 {
			fatal("usage: adapter bench <workload> <operations> <warmup-batches> <measured-batches>")
		}
		operations := positive(os.Args[3])
		warmup := nonnegative(os.Args[4])
		samples := positive(os.Args[5])
		if err := a.bench(os.Args[2], operations, warmup, samples); err != nil {
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
			return nil, fmt.Errorf("read fixture %s: %w", name, err)
		}
		fixtures[name] = contents
	}
	return &adapter{fixtures: fixtures}, nil
}

func (a *adapter) verify() error {
	ctx := context.Background()
	client := newClient()
	var message *starlings.MessageCreate
	client.On(func(event *starlings.MessageCreate) { message = event })
	if err := client.HandleGatewayFrame(ctx, a.fixtures["message-create.json"]); err != nil {
		return fmt.Errorf("message dispatch: %w", err)
	}
	if message == nil || message.ID != 1234567890123456789 || message.ChannelID != 987654321098765432 ||
		message.GuildID != 111111111111111111 || message.Author == nil || message.Author.ID != 222222222222222222 || message.Content != "hello there" {
		return errors.New("message callback did not match canonical fixture")
	}
	if err := client.HandleGatewayFrame(ctx, a.fixtures["guild-create-500.json"]); err != nil {
		return fmt.Errorf("guild dispatch: %w", err)
	}
	guild, ok := client.State.Guild(guildID)
	members, roles := client.State.Members(guildID), client.State.Roles(guildID)
	if !ok || guild.OwnerID != 80351110224678912 || len(members) != 500 || len(roles) != 40 || len(guild.Channels) != 25 {
		return fmt.Errorf("state mismatch: found=%t owner=%d members=%d roles=%d channels=%d",
			ok, guild.OwnerID, len(members), len(roles), len(guild.Channels))
	}
	for _, id := range []starlings.Snowflake{firstUser, firstUser + 250, firstUser + 499} {
		if member, found := client.State.Member(guildID, id); !found || member.User == nil || member.User.ID != id {
			return fmt.Errorf("member %d missing from state", id)
		}
	}
	permissions, err := client.State.Permissions(guildID, channelID, firstUser)
	if err != nil || permissions != 76800 {
		return fmt.Errorf("permissions=%d err=%v", permissions, err)
	}
	before := message
	if err := client.HandleGatewayFrame(ctx, a.fixtures["malformed-frame.json"]); err == nil {
		return errors.New("malformed frame did not return an error")
	}
	if message != before {
		return errors.New("malformed frame invoked a typed callback")
	}
	return nil
}

func (a *adapter) bench(workload string, operations, warmup, samples int) error {
	run, coverage, callbacks, err := a.workload(workload)
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
		checksum, callbackCount, err := run(operations)
		elapsed := time.Since(started)
		if err != nil {
			return err
		}
		if callbacks && callbackCount != uint64(operations) {
			return fmt.Errorf("callback count=%d want=%d", callbackCount, operations)
		}
		write(result{
			Schema: 1, Library: "starlings", Language: "Go", Commit: commit(), Runtime: runtime.Version(), Build: "release",
			Workload: workload, Coverage: coverage, Sample: sample, Operations: operations,
			ElapsedNS: elapsed.Nanoseconds(), NSPerOp: float64(elapsed.Nanoseconds()) / float64(operations),
			Callbacks: callbackCount, Digest: digest(fmt.Sprintf("%d", checksum)),
		})
	}
	return nil
}

func (a *adapter) workload(name string) (func(int) (uint64, uint64, error), string, bool, error) {
	ctx := context.Background()
	switch name {
	case "message_handled":
		client := newClient()
		var checksum, callbacks uint64
		client.On(func(event *starlings.MessageCreate) {
			callbacks++
			checksum = checksum*1099511628211 + uint64(event.ID) + uint64(event.Author.ID)
		})
		return func(operations int) (uint64, uint64, error) {
			checksum = 1469598103934665603
			startCallbacks := callbacks
			for range operations {
				if err := client.HandleGatewayFrame(ctx, a.fixtures["message-create.json"]); err != nil {
					return 0, 0, err
				}
			}
			return checksum, callbacks - startCallbacks, nil
		}, "full_dispatch", true, nil
	case "message_unhandled":
		client := newClient()
		return func(operations int) (uint64, uint64, error) {
			for range operations {
				if err := client.HandleGatewayFrame(ctx, a.fixtures["message-create.json"]); err != nil {
					return 0, 0, err
				}
			}
			message, ok := client.State.Message(987654321098765432, 1234567890123456789)
			return uint64(message.ID), 0, boolError(ok, "message was not consumed by internal state")
		}, "state_update", false, nil
	case "guild_create_state":
		client := newClient()
		return func(operations int) (uint64, uint64, error) {
			for range operations {
				if err := client.HandleGatewayFrame(ctx, a.fixtures["guild-create-500.json"]); err != nil {
					return 0, 0, err
				}
			}
			members := client.State.Members(guildID)
			if len(members) != 500 {
				return 0, 0, fmt.Errorf("member count=%d", len(members))
			}
			return uint64(members[0].User.ID) ^ uint64(members[len(members)-1].User.ID), 0, nil
		}, "state_update", false, nil
	case "member_lookup":
		client := newClient()
		if err := client.HandleGatewayFrame(ctx, a.fixtures["guild-create-500.json"]); err != nil {
			return nil, "", false, err
		}
		ids := [...]starlings.Snowflake{firstUser, firstUser + 250, firstUser + 499, firstUser + 9999}
		return func(operations int) (uint64, uint64, error) {
			checksum := uint64(1469598103934665603)
			for i := range operations {
				member, ok := client.State.Member(guildID, ids[i&3])
				if ok {
					checksum = checksum*1099511628211 + uint64(member.User.ID)
				}
			}
			return checksum, 0, nil
		}, "public_cache", false, nil
	case "permission_resolve":
		client := newClient()
		if err := client.HandleGatewayFrame(ctx, a.fixtures["guild-create-500.json"]); err != nil {
			return nil, "", false, err
		}
		return func(operations int) (uint64, uint64, error) {
			checksum := uint64(1469598103934665603)
			for range operations {
				permissions, err := client.State.Permissions(guildID, channelID, firstUser)
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

func newClient() *starlings.Client {
	return starlings.New("offline",
		starlings.WithAsyncEvents(false),
		starlings.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
}

func boolError(ok bool, message string) error {
	if ok {
		return nil
	}
	return errors.New(message)
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func commit() string {
	if value := os.Getenv("ARENA_COMMIT"); value != "" {
		return value
	}
	return "working-tree"
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

func write(value any) {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
