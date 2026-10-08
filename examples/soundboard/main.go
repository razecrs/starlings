// Command soundboard is a Discord soundboard bot built on starlings.
//
// Type /soundboard and it posts an embed listing every sound in the server,
// with a button per sound. Press one and the bot joins your voice channel and
// plays it.
//
// The neat part: playing a soundboard sound needs no voice websocket at all.
// The bot only has to be *present* in the channel - a gateway voice-state
// update - and Discord mixes the audio server-side. So this is a working voice
// feature with none of the voice protocol.
//
//	DISCORD_TOKEN=... go run ./examples/soundboard
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/razecrs/starlings"
)

// buttonsPerRow is Discord's limit; five rows of five gives 25 sounds a page.
const (
	buttonsPerRow = 5
	maxRows       = 5
	soundsPerPage = buttonsPerRow * maxRows
)

// voiceTracker remembers which voice channel each user is in, so pressing a
// button can put the bot where the presser is. Discord only tells us this
// through VOICE_STATE_UPDATE, so we accumulate it as events arrive.
type voiceTracker struct {
	mu sync.RWMutex
	in map[starlings.Snowflake]starlings.Snowflake // user -> channel
}

func newVoiceTracker() *voiceTracker {
	return &voiceTracker{in: make(map[starlings.Snowflake]starlings.Snowflake)}
}

func (v *voiceTracker) set(userID, channelID starlings.Snowflake) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if channelID.IsZero() {
		delete(v.in, userID)
		return
	}
	v.in[userID] = channelID
}

func (v *voiceTracker) get(userID starlings.Snowflake) (starlings.Snowflake, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	ch, ok := v.in[userID]
	return ch, ok
}

func main() {
	log.SetFlags(log.Ltime)

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("set DISCORD_TOKEN")
	}
	guildID, err := starlings.ParseSnowflake(os.Getenv("SOUNDBOARD_GUILD_ID"))
	if err != nil || guildID.IsZero() {
		log.Fatal("set SOUNDBOARD_GUILD_ID to the server used for command registration")
	}

	// GuildVoiceStates is what makes VOICE_STATE_UPDATE arrive; without it the
	// bot can never tell where anyone is. It is not privileged.
	bot := starlings.New(starlings.WithToken(token),
		starlings.WithIntents(starlings.IntentGuilds|starlings.IntentGuildVoiceStates),
		starlings.WithStatus(starlings.StatusOnline, starlings.Listening("/soundboard")),
	)

	voices := newVoiceTracker()
	var autoplayOnce sync.Once
	var boardOnce sync.Once

	bot.On(func(e *starlings.VoiceStateUpdate) {
		voices.set(e.UserID, e.ChannelID)
	})

	// A guild's voice states arrive in bulk on GUILD_CREATE, so the tracker is
	// populated for people who were already connected before the bot started.
	bot.On(func(g *starlings.GuildCreate) {
		for _, vs := range g.VoiceStates {
			voices.set(vs.UserID, vs.ChannelID)
		}
		log.Printf("ready in %q (%d already in voice)", g.Name, len(g.VoiceStates))
		for _, channel := range g.Channels {
			if channel.Type == starlings.ChannelGuildText && strings.EqualFold(channel.Name, "（👾）bot-cmd") {
				channelID := channel.ID
				boardOnce.Do(func() { go postBoard(bot, g.ID, channelID) })
				break
			}
		}
		if os.Getenv("SOUNDBOARD_AUTOPLAY") == "1" {
			autoplayOnce.Do(func() { go autoplay(bot, g) })
		}
	})

	bot.Slash("soundboard", "Show the soundboard", func(i *starlings.InteractionCreate) {
		showBoard(bot, i)
	})

	// Buttons come back as component interactions, routed by custom ID rather
	// than by command name.
	bot.On(func(i *starlings.InteractionCreate) {
		if i.Type != starlings.InteractionMessageComponent {
			return
		}
		handleButton(bot, i, voices)
	})

	bot.On(func(r *starlings.Ready) {
		log.Printf("online as %s", r.User.Tag())
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := bot.SyncCommands(ctx, guildID); err != nil {
			log.Printf("registering /soundboard: %v", err)
			return
		}
		log.Printf("/soundboard registered in guild %s", guildID)
	})

	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}

// autoplay is handy for checking voice without registering the slash command.
// It joins the first occupied channel and plays every server sound once.
func autoplay(bot *starlings.Client, guild *starlings.GuildCreate) {
	self := bot.Self()
	var channelID starlings.Snowflake
	for _, state := range guild.VoiceStates {
		if !state.ChannelID.IsZero() && (self == nil || state.UserID != self.ID) {
			channelID = state.ChannelID
			break
		}
	}
	if channelID.IsZero() {
		log.Print("autoplay: nobody is in voice")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sounds, err := bot.GuildSoundboardSounds(ctx, guild.ID)
	if err != nil {
		log.Printf("autoplay: reading sounds: %v", err)
		return
	}
	if len(sounds) == 0 {
		log.Print("autoplay: this server has no sounds")
		return
	}
	if err := bot.JoinVoice(ctx, guild.ID, channelID, false, false); err != nil {
		log.Printf("autoplay: joining voice: %v", err)
		return
	}
	time.Sleep(600 * time.Millisecond)
	for i, sound := range sounds {
		if err := bot.SendSoundboardSound(ctx, channelID, sound.SoundID, 0); err != nil {
			log.Printf("autoplay: playing %q: %v", sound.Name, err)
			continue
		}
		log.Printf("autoplay: played %q (%d/%d)", sound.Name, i+1, len(sounds))
		if i+1 < len(sounds) {
			time.Sleep(5 * time.Second)
		}
	}
	log.Printf("autoplay: finished %d sounds", len(sounds))
}

func postBoard(bot *starlings.Client, guildID, channelID starlings.Snowflake) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sounds, err := bot.GuildSoundboardSounds(ctx, guildID)
	if err != nil {
		log.Printf("posting soundboard: %v", err)
		return
	}
	if len(sounds) > soundsPerPage {
		sounds = sounds[:soundsPerPage]
	}
	_, err = bot.SendComplex(ctx, channelID, starlings.SendData{
		Embeds:          []starlings.Embed{boardEmbed(sounds)},
		Components:      boardButtons(sounds),
		AllowedMentions: starlings.NoMentions(),
	})
	if err != nil {
		log.Printf("posting soundboard: %v", err)
		return
	}
	log.Print("soundboard posted in （👾）bot-cmd")
}

// showBoard answers the slash command with the sound list and its buttons.
func showBoard(bot *starlings.Client, i *starlings.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sounds, err := bot.GuildSoundboardSounds(ctx, i.GuildID)
	if err != nil {
		i.ReplyEphemeral("Could not read the soundboard: " + err.Error())
		return
	}
	if len(sounds) == 0 {
		i.ReplyEphemeral("This server has no soundboard sounds yet. " +
			"Add some in Server Settings → Soundboard.")
		return
	}
	if len(sounds) > soundsPerPage {
		sounds = sounds[:soundsPerPage]
	}

	if err := i.ReplyComplex(starlings.InteractionResponseData{
		Embeds:     []starlings.Embed{boardEmbed(sounds)},
		Components: boardButtons(sounds),
	}); err != nil {
		log.Printf("showing the board: %v", err)
	}
}

// boardEmbed renders the sound list.
func boardEmbed(sounds []starlings.SoundboardSound) starlings.Embed {
	var b strings.Builder
	for n, s := range sounds {
		emoji := s.Emoji()
		if emoji == "" {
			emoji = "🔊"
		}
		fmt.Fprintf(&b, "%s  **%s**\n", emoji, s.Name)
		_ = n
	}

	e := starlings.Embed{
		Title:       "🎛️  Soundboard",
		Description: b.String(),
		Color:       0x5865F2,
	}
	return *e.SetFooter(
		fmt.Sprintf("%d sound%s · join a voice channel, then press a button",
			len(sounds), plural(len(sounds))), "")
}

// boardButtons lays the sounds out as rows of buttons, encoding each sound's
// ID into the custom ID so the press can be routed back to it.
func boardButtons(sounds []starlings.SoundboardSound) []starlings.Component {
	var rows []starlings.Component

	for start := 0; start < len(sounds); start += buttonsPerRow {
		end := min(start+buttonsPerRow, len(sounds))

		var row []starlings.Component
		for _, s := range sounds[start:end] {
			btn := starlings.Button(starlings.ButtonSecondary,
				truncate(s.Name, 20), "play:"+s.SoundID.String())

			// A custom emoji needs its ID; a unicode one just needs the rune.
			if !s.EmojiID.IsZero() {
				btn.Emoji = &starlings.Emoji{ID: s.EmojiID, Name: s.EmojiName}
			} else if s.EmojiName != "" {
				btn.Emoji = &starlings.Emoji{Name: s.EmojiName}
			}
			row = append(row, btn)
		}
		rows = append(rows, starlings.ActionRow(row...))
	}
	return rows
}

// handleButton plays the sound a button refers to.
func handleButton(bot *starlings.Client, i *starlings.InteractionCreate, voices *voiceTracker) {
	soundID, ok := strings.CutPrefix(i.Data.CustomID, "play:")
	if !ok {
		return
	}
	id, err := starlings.ParseSnowflake(soundID)
	if err != nil {
		i.ReplyEphemeral("That button is malformed.")
		return
	}

	user := i.Invoker()
	if user == nil {
		i.ReplyEphemeral("Could not work out who pressed that.")
		return
	}

	channelID, inVoice := voices.get(user.ID)
	if !inVoice {
		i.ReplyEphemeral("Join a voice channel first, then press the button.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := i.Respond(ctx, starlings.InteractionResponse{
		Type: starlings.CallbackDeferredUpdateMessage,
	}); err != nil {
		log.Printf("acknowledging the press: %v", err)
		return
	}

	// Soundboard effects are rejected while the bot is muted or deafened.
	if err := bot.JoinVoice(ctx, i.GuildID, channelID, false, false); err != nil {
		followupError(i, ctx, "Could not join your voice channel: "+err.Error())
		return
	}

	// Discord needs a moment to register the bot's presence before it will
	// accept a sound for that channel.
	time.Sleep(600 * time.Millisecond)

	if err := bot.SendSoundboardSound(ctx, channelID, id, 0); err != nil {
		followupError(i, ctx, "Could not play that: "+err.Error())
		return
	}
}

func followupError(i *starlings.InteractionCreate, ctx context.Context, message string) {
	_, err := i.Followup(ctx, starlings.InteractionResponseData{
		Content: message,
		Flags:   starlings.MessageFlagEphemeral,
	})
	if err != nil {
		log.Printf("sending button error: %v", err)
	}
}

func truncate(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return strings.TrimSpace(string(r[:maxLen-1])) + "…"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
