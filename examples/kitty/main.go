// Command kitty runs a live smoke test against Discord and posts the results
// in a channel named dev-chat.
//
//	DISCORD_TOKEN=... go run ./examples/kitty
//
// Set DEV_CHAT_ID if more than one server has a dev-chat channel.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/razecrs/starlings"
)

const smokeNonce = "kitty-smoke"

type smokeTest struct {
	bot       *starlings.Client
	channelID starlings.Snowflake
	guildID   starlings.Snowflake
	reports   chan string

	membersSeen atomic.Bool
	soundsSeen  atomic.Bool
	channelSeen atomic.Bool
}

func main() {
	loadDotEnv()
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("set DISCORD_TOKEN or add it to .env")
	}

	bot := starlings.New(starlings.WithToken(token),
		starlings.WithIntents(starlings.IntentGuilds),
		starlings.WithStatus(starlings.StatusOnline, starlings.Listening("starlings tests")),
	)

	var once sync.Once
	var test *smokeTest

	bot.On(func(g *starlings.GuildCreate) {
		channelID := devChannel(g)
		if channelID.IsZero() {
			return
		}
		once.Do(func() {
			test = newSmokeTest(bot, g.ID, channelID)
			test.report("kitty smoke test starting in **%s**", g.Name)
			test.report("GUILD_CREATE decoded: %d channels, %d members, %d voice states", len(g.Channels), len(g.Members), len(g.VoiceStates))
			go test.run()
		})
	})

	bot.On(func(e *starlings.GuildMembersChunk) {
		if test == nil || e.GuildID != test.guildID || e.Nonce != smokeNonce {
			return
		}
		test.membersSeen.Store(true)
		test.report("✅ GUILD_MEMBERS_CHUNK: %d member(s), chunk %d/%d", len(e.Members), e.ChunkIndex+1, e.ChunkCount)
	})

	bot.On(func(e *starlings.SoundboardSounds) {
		if test == nil || e.GuildID != test.guildID {
			return
		}
		test.soundsSeen.Store(true)
		test.report("✅ SOUNDBOARD_SOUNDS: %d sound(s)", len(e.SoundboardSounds))
	})

	bot.On(func(e *starlings.ChannelInfo) {
		if test == nil || e.GuildID != test.guildID {
			return
		}
		test.channelSeen.Store(true)
		for _, channel := range e.Channels {
			name := channel.ID.String()
			if cached, ok := bot.State.Channel(channel.ID); ok && cached.Name != "" {
				name = cached.Name
			}
			test.report("✅ CHANNEL_INFO for #%s: status=%v, voice_start_time=%v", name, channel.Status, channel.VoiceStartTime)
		}
	})

	bot.On(func(e *starlings.RateLimited) {
		if test == nil {
			return
		}
		test.report("⚠️ gateway rate limit: op %d, retry in %.2fs", e.Opcode, e.RetryAfter)
	})

	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}

func newSmokeTest(bot *starlings.Client, guildID, channelID starlings.Snowflake) *smokeTest {
	s := &smokeTest{
		bot:       bot,
		guildID:   guildID,
		channelID: channelID,
		reports:   make(chan string, 32),
	}
	go s.postReports()
	return s
}

func (s *smokeTest) run() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.bot.UpdatePresence(ctx, starlings.Presence{
		Status:     starlings.StatusOnline,
		Activities: []starlings.Activity{starlings.Playing("with gateway controls")},
	}); err != nil {
		s.report("❌ UpdatePresence: %v", err)
	} else {
		s.report("✅ UpdatePresence sent")
	}

	self := s.bot.Self()
	if self == nil {
		s.report("❌ member request: bot user is missing after READY")
	} else {
		err := s.bot.RequestMembers(ctx, starlings.MemberRequest{
			GuildID: s.guildID,
			UserIDs: []starlings.Snowflake{self.ID},
			Nonce:   smokeNonce,
		})
		s.sent("RequestMembers", err)
	}

	s.sent("RequestSoundboardSounds", s.bot.RequestSoundboardSounds(ctx, s.guildID))
	s.sent("RequestChannelInfo", s.bot.RequestChannelInfo(ctx, s.guildID,
		starlings.ChannelInfoStatus,
		starlings.ChannelInfoVoiceStartTime,
	))

	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	<-timer.C

	if !s.membersSeen.Load() {
		s.report("❌ no GUILD_MEMBERS_CHUNK received")
	}
	if !s.soundsSeen.Load() {
		s.report("❌ no SOUNDBOARD_SOUNDS received")
	}
	if !s.channelSeen.Load() {
		s.report("❌ no CHANNEL_INFO received")
	}
	if s.membersSeen.Load() && s.soundsSeen.Load() && s.channelSeen.Load() {
		s.report("all gateway response checks passed 🐈")
	}
}

func (s *smokeTest) sent(name string, err error) {
	if err != nil {
		s.report("❌ %s: %v", name, err)
		return
	}
	s.report("↗️ %s sent", name)
}

func (s *smokeTest) report(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	s.reports <- msg
}

func (s *smokeTest) postReports() {
	for msg := range s.reports {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := s.bot.SendComplex(ctx, s.channelID, starlings.SendData{
			Content:         msg,
			AllowedMentions: starlings.NoMentions(),
		})
		cancel()
		if err != nil {
			log.Printf("posting to dev-chat: %v", err)
		}
	}
}

func devChannel(g *starlings.GuildCreate) starlings.Snowflake {
	if raw := os.Getenv("DEV_CHAT_ID"); raw != "" {
		id, err := starlings.ParseSnowflake(raw)
		if err != nil {
			log.Printf("bad DEV_CHAT_ID: %v", err)
			return 0
		}
		for _, channel := range g.Channels {
			if channel.ID == id {
				return id
			}
		}
		return 0
	}

	for _, channel := range g.Channels {
		if channel.Type == starlings.ChannelGuildText && channel.Name == "dev-chat" {
			return channel.ID
		}
	}
	return 0
}

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.HasPrefix(key, "#") {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			_ = os.Setenv(key, value)
		}
	}
}
