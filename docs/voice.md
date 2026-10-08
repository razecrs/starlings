# Voice and soundboard

Voice lives in its own package, so bots without voice never link its
dependencies:

```go
import "github.com/razecrs/starlings/voice"
```

## Requirements

The bot needs `Connect` and `Speak` in the target channel. Include `IntentGuildVoiceStates`; joining depends on `VOICE_STATE_UPDATE` and `VOICE_SERVER_UPDATE` from the main gateway.

`voice.Connect` waits for those events. Run it from a goroutine if the trigger is a gateway handler.

```go
go func() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := voice.Connect(ctx, bot, guildID, channelID)
	if err != nil {
		log.Print(err)
		return
	}
	defer conn.Close(context.Background())

	track, err := conn.PlayFile(context.Background(), "song.mp3")
	if err != nil {
		log.Print(err)
		return
	}
	defer track.Close()
	if err := track.Wait(); err != nil {
		log.Print(err)
	}
}()
```

The join completes the voice v8 websocket, UDP discovery, AEAD transport negotiation, and DAVE setup before returning.

## FFmpeg playback

`PlayFile` accepts any local input FFmpeg can decode, including the audio track of a video. FFmpeg must be installed and available on `PATH`. Starlings maps the first audio stream, removes video, and produces stereo 48 kHz Opus in 20 ms frames.

The context owns the FFmpeg process. Cancel it to stop early. `Wait` reports decoder/process errors; `Close` is safe after completion. Starting another provider with `Play` replaces and closes the previous provider.

Bot accounts can publish audio but cannot publish camera or Go Live video through Discord's bot API.

## Supply Opus directly

Implement `voice.OpusProvider` when the application already has Discord-ready frames:

```go
type source struct { /* decoder state */ }

func (s *source) ProvideOpusFrame() ([]byte, error) {
	// Return one stereo, 48 kHz, 20 ms Opus frame.
	// Return io.EOF at the end.
}

func (s *source) Close() {}

if err := conn.Play(&source{}); err != nil {
	log.Fatal(err)
}
```

Do not return Ogg pages or an `OpusHead`; return the encoded Opus packet payload for one frame.

## Receive packets

```go
err := conn.Receive(func(packet *voice.OpusPacket) error {
	copyForWorker := append([]byte(nil), packet.Opus...)
	go consume(packet.UserID, copyForWorker)
	return nil
})
```

The callback runs on the voice receive loop. Hand decoding, storage, speech recognition, and network work to another goroutine. Packet audio remains Opus-encoded.

## Soundboard

`GuildSoundboardSounds` lists a guild's sounds. `SendSoundboardSound` asks Discord to play one in a channel. Discord mixes the sound server-side; the bot must be present in voice, but soundboard playback itself does not stream an Opus track.

`RequestSoundboardSounds` is the gateway request form. Its result arrives as a `SoundboardSounds` event. See [`examples/soundboard`](../examples/soundboard/main.go) for pagination, button routing, and voice-state tracking.

## Shutdown and failures

`Connection.Close` stops playback and leaves the channel. `Client.Close` also closes the voice manager. Treat context cancellation as an expected stop; log handshake, permission, DAVE, and FFmpeg errors because reconnecting the main gateway cannot repair a missing channel permission or missing executable.
