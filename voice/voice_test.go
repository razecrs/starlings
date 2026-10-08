package voice

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	discordvoice "github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"

	"github.com/razecrs/starlings"
)

func TestConnectValidatesIDsAndReadiness(t *testing.T) {
	c := starlings.New(starlings.WithToken("token"))
	if _, err := Connect(context.Background(), c, 0, 2); err == nil {
		t.Fatal("Connect accepted an empty guild ID")
	}
	if _, err := Connect(context.Background(), c, 1, 0); err == nil {
		t.Fatal("Connect accepted an empty channel ID")
	}
	if _, err := Connect(context.Background(), c, 1, 2); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Connect before READY = %v, want ErrNotReady", err)
	}
}

func TestVoiceEventConversion(t *testing.T) {
	state := toVoiceStateUpdate(&starlings.VoiceStateUpdate{VoiceState: starlings.VoiceState{
		GuildID: 1, ChannelID: 2, UserID: 3, SessionID: "session", SelfMute: true,
	}})
	if state.GuildID != 1 || state.ChannelID == nil || *state.ChannelID != 2 || state.UserID != 3 || !state.SelfMute {
		t.Fatalf("bad voice state conversion: %#v", state)
	}

	left := toVoiceStateUpdate(&starlings.VoiceStateUpdate{VoiceState: starlings.VoiceState{GuildID: 1}})
	if left.ChannelID != nil {
		t.Fatalf("disconnected channel = %v, want nil", left.ChannelID)
	}

	server := toVoiceServerUpdate(&starlings.VoiceServerUpdate{GuildID: 1, Token: "token", Endpoint: "voice.test"})
	if server.Endpoint == nil || *server.Endpoint != "voice.test" || server.Token != "token" {
		t.Fatalf("bad voice server conversion: %#v", server)
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
