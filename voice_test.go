package starlings

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	discordvoice "github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
)

func TestConnectVoiceValidatesIDsAndReadiness(t *testing.T) {
	c := New("token")
	if _, err := c.ConnectVoice(context.Background(), 0, 2); err == nil {
		t.Fatal("ConnectVoice accepted an empty guild ID")
	}
	if _, err := c.ConnectVoice(context.Background(), 1, 0); err == nil {
		t.Fatal("ConnectVoice accepted an empty channel ID")
	}
	if _, err := c.ConnectVoice(context.Background(), 1, 2); !errors.Is(err, errNotReady) {
		t.Fatalf("ConnectVoice before READY = %v, want errNotReady", err)
	}
}

func TestVoiceEventConversion(t *testing.T) {
	state := toVoiceStateUpdate(&VoiceStateUpdate{VoiceState: VoiceState{
		GuildID: 1, ChannelID: 2, UserID: 3, SessionID: "session", SelfMute: true,
	}})
	if state.GuildID != 1 || state.ChannelID == nil || *state.ChannelID != 2 || state.UserID != 3 || !state.SelfMute {
		t.Fatalf("bad voice state conversion: %#v", state)
	}

	left := toVoiceStateUpdate(&VoiceStateUpdate{VoiceState: VoiceState{GuildID: 1}})
	if left.ChannelID != nil {
		t.Fatalf("disconnected channel = %v, want nil", left.ChannelID)
	}

	server := toVoiceServerUpdate(&VoiceServerUpdate{GuildID: 1, Token: "token", Endpoint: "voice.test"})
	if server.Endpoint == nil || *server.Endpoint != "voice.test" || server.Token != "token" {
		t.Fatalf("bad voice server conversion: %#v", server)
	}
}

func TestVoiceBookkeepingUsesOrderedInternalHandlers(t *testing.T) {
	c := New("token", WithAsyncEvents(true))
	c.self.Store(&User{ID: 1})
	before := make(map[string]int, 2)
	for _, name := range []string{"VOICE_STATE_UPDATE", "VOICE_SERVER_UPDATE"} {
		if slot := c.slotFor(name); slot != nil {
			before[name] = slot.internal
		}
	}
	manager, err := c.voiceManager()
	if err != nil {
		t.Fatalf("voiceManager: %v", err)
	}
	defer manager.close(context.Background())

	for _, name := range []string{"VOICE_STATE_UPDATE", "VOICE_SERVER_UPDATE"} {
		slot := c.slotFor(name)
		if slot == nil || slot.internal != before[name]+1 {
			t.Fatalf("%s internal handlers = %v, want %d", name, slot, before[name]+1)
		}
	}
}

func TestFFmpegProviderReadsOggPackets(t *testing.T) {
	data := append(oggTestPage([][]byte{[]byte("OpusHead"), []byte("OpusTags"), {1, 2, 3}}),
		oggTestContinuedPacket(bytes.Repeat([]byte{4}, 300))...)
	p := &FFmpegOpusProvider{reader: bufio.NewReader(bytes.NewReader(data))}

	frame, err := p.ProvideOpusFrame()
	if err != nil || !bytes.Equal(frame, []byte{1, 2, 3}) {
		t.Fatalf("first frame = %v, %v", frame, err)
	}
	frame, err = p.ProvideOpusFrame()
	if err != nil || len(frame) != 300 || !bytes.Equal(frame, bytes.Repeat([]byte{4}, 300)) {
		t.Fatalf("continued frame length = %d, err = %v", len(frame), err)
	}
	if _, err = p.ProvideOpusFrame(); !errors.Is(err, io.EOF) {
		t.Fatalf("end of stream = %v, want io.EOF", err)
	}
}

func TestFFmpegProviderRejectsInvalidOgg(t *testing.T) {
	p := &FFmpegOpusProvider{reader: bufio.NewReader(bytes.NewReader(make([]byte, 27)))}
	if _, err := p.ProvideOpusFrame(); err == nil {
		t.Fatal("invalid Ogg stream was accepted")
	}
}

func TestOpusReceiverCopiesPacket(t *testing.T) {
	original := []byte{1, 2, 3}
	var got *OpusPacket
	receiver := opusReceiver{callback: func(packet *OpusPacket) error {
		got = packet
		return nil
	}}
	err := receiver.ReceiveOpusFrame(snowflake.ID(9), &discordvoice.Packet{
		Sequence: 2, Timestamp: 3, SSRC: 4, Opus: original,
	})
	original[0] = 99
	if err != nil || got == nil || got.UserID != 9 || got.Sequence != 2 || got.Timestamp != 3 || got.SSRC != 4 || !bytes.Equal(got.Opus, []byte{1, 2, 3}) {
		t.Fatalf("received packet = %#v, err = %v", got, err)
	}
}

func oggTestPage(packets [][]byte) []byte {
	header := make([]byte, 27)
	copy(header, "OggS")
	lacing := make([]byte, 0, len(packets))
	var body []byte
	for _, packet := range packets {
		if len(packet) >= 255 {
			panic("oggTestPage only accepts short packets")
		}
		lacing = append(lacing, byte(len(packet)))
		body = append(body, packet...)
	}
	header[26] = byte(len(lacing))
	return append(append(header, lacing...), body...)
}

func oggTestContinuedPacket(packet []byte) []byte {
	firstHeader := make([]byte, 27)
	copy(firstHeader, "OggS")
	firstHeader[26] = 1
	first := append(append(firstHeader, 255), packet[:255]...)

	secondHeader := make([]byte, 27)
	copy(secondHeader, "OggS")
	secondHeader[5] = 1 // continued packet flag
	secondHeader[26] = 1
	second := append(append(secondHeader, byte(len(packet)-255)), packet[255:]...)
	return append(first, second...)
}
