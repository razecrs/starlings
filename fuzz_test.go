package starlings

import (
	"context"
	"crypto/ed25519"
	"net/url"
	"strings"
	"testing"
)

func FuzzGatewayFrameNeverPanics(f *testing.F) {
	for _, seed := range [][]byte{
		{}, []byte(`{`), []byte(messageCreateFrame),
		[]byte(`{"op":0,"t":"UNKNOWN","s":1,"d":{"x":true}}`),
		[]byte(`{"d":{},"s":1,"t":"READY","op":0}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, frame []byte) {
		if len(frame) > 1<<20 {
			t.Skip()
		}
		client := testClient()
		On(client, func(*MessageCreate) {})
		var gateway gateway
		_ = gateway.handleFrame(context.Background(), client, frame)
	})
}

func FuzzVerifyInteractionNeverPanics(f *testing.F) {
	f.Add([]byte("short"), "bad", "0", []byte(`{"type":1}`))
	f.Add(make([]byte, ed25519.PublicKeySize), strings.Repeat("00", ed25519.SignatureSize), "1700000000", []byte{})
	f.Fuzz(func(t *testing.T, key []byte, signature, timestamp string, body []byte) {
		if len(key) > 1<<10 || len(signature) > 1<<12 || len(timestamp) > 1<<10 || len(body) > 1<<20 {
			t.Skip()
		}
		_ = VerifyInteraction(ed25519.PublicKey(key), signature, timestamp, body)
	})
}

func FuzzMultipartFilenameHeader(f *testing.F) {
	for _, seed := range []string{"cat.png", `quote\".png`, "line\r\nX-Evil: yes.png", "💫.webp"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 4096 {
			t.Skip()
		}
		header := filePartHeader(0, File{Name: name})
		for _, values := range header {
			for _, value := range values {
				if strings.ContainsAny(value, "\r\n") {
					t.Fatalf("filename injected a MIME header: %q", value)
				}
			}
		}
	})
}

func FuzzSensitivePathRedaction(f *testing.F) {
	f.Add("webhooks", "123", "secret", "messages/456")
	f.Add("interactions", "abc", "tok.en", "callback")
	f.Fuzz(func(t *testing.T, family, id, secret, suffix string) {
		if (family != "webhooks" && family != "interactions") || id == "" || secret == "" ||
			len(id)+len(secret)+len(suffix) > 8192 || strings.ContainsAny(id+secret+suffix, "/?#") {
			t.Skip()
		}
		path := "/" + family + "/" + id + "/" + secret + "/" + suffix
		got := safeRequestPath(path)
		if got == "[redacted path]" {
			return
		}
		parsed, err := url.Parse(got)
		if err != nil {
			t.Fatalf("redacted path is invalid: %v", err)
		}
		parts := strings.Split(parsed.Path, "/")
		if len(parts) >= 4 && parts[3] == secret {
			t.Fatalf("credential segment %q survived redaction in %q", secret, got)
		}
	})
}
