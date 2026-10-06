package starlings

import (
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestComponentsV2AndPollModels(t *testing.T) {
	data := ComponentsMessage(Container(
		TextDisplay("hello"),
		Section(Thumbnail("https://cdn.example/cat.png", "cat"), TextDisplay("world")),
	))
	if data.Flags&MessageFlagIsComponentsV2 == 0 {
		t.Fatal("ComponentsMessage did not set the V2 flag")
	}

	poll := NewPoll("best?", "cats", "more cats")
	poll.Duration = 24
	data.Poll = &poll
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip SendData
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if got := roundTrip.Components[0].Components[1].Accessory.Media.URL; got != "https://cdn.example/cat.png" {
		t.Fatalf("thumbnail URL = %q", got)
	}
	if roundTrip.Poll.Answers[1].Media.Text != "more cats" {
		t.Fatal("poll answers did not round trip")
	}

	var submitted Component
	if err := json.Unmarshal([]byte(`{"type":23,"id":7,"custom_id":"tos","value":true}`), &submitted); err != nil {
		t.Fatal(err)
	}
	if checked, ok := submitted.BoolValue(); !ok || !checked {
		t.Fatalf("checkbox value = %#v", submitted.Value)
	}
}

func TestModernMessageAndInteractionModels(t *testing.T) {
	const payload = `{
		"id":"10","channel_id":"20","type":0,"flags":32768,
		"poll":{"question":{"text":"q"},"answers":[{"answer_id":1,"poll_media":{"text":"a"}}],"layout_type":1},
		"interaction_metadata":{"id":"30","type":2,"authorizing_integration_owners":{"1":"40"}},
		"message_snapshots":[{"message":{"id":"11","channel_id":"20","content":"forwarded","type":0}}]
	}`
	var m Message
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatal(err)
	}
	if m.Flags&MessageFlagIsComponentsV2 == 0 || m.Poll.Answers[0].Media.Text != "a" {
		t.Fatal("modern message fields were not decoded")
	}
	if m.InteractionMeta.AuthorizingIntegrationOwners["1"] != 40 {
		t.Fatal("authorizing owner was not decoded")
	}
	if m.Snapshots[0].Message.Content != "forwarded" {
		t.Fatal("message snapshot was not decoded")
	}

	i := Interaction{AuthorizingIntegrationOwners: map[string]Snowflake{"1": 40}}
	if id, ok := i.AuthorizingOwner(IntegrationUserInstall); !ok || id != 40 {
		t.Fatalf("authorizing owner = %v, %v", id, ok)
	}
}

func TestVoiceChannelMetadataEventsUpdateState(t *testing.T) {
	c := New("test")
	c.dispatch("CHANNEL_CREATE", []byte(`{"id":"20","guild_id":"10","name":"voice","type":2}`))
	c.dispatch("CHANNEL_INFO", []byte(`{"guild_id":"10","channels":[{"id":"20","status":"gaming","voice_start_time":100}]}`))
	channel, ok := c.State.Channel(20)
	if !ok || channel.Status != "gaming" || channel.VoiceStartTime == nil || channel.VoiceStartTime.Unix() != 100 {
		t.Fatalf("channel info did not update state: %#v, %v", channel, ok)
	}
	c.dispatch("VOICE_CHANNEL_STATUS_UPDATE", []byte(`{"id":"20","guild_id":"10","status":"music"}`))
	channel, _ = c.State.Channel(20)
	if channel.Status != "music" {
		t.Fatalf("voice status = %q", channel.Status)
	}
	c.dispatch("VOICE_CHANNEL_START_TIME_UPDATE", []byte(`{"id":"20","guild_id":"10","voice_start_time":null}`))
	channel, _ = c.State.Channel(20)
	if channel.VoiceStartTime != nil {
		t.Fatal("voice start time was not cleared")
	}
}

func TestHandlerLifecycleRawAndSubscriptions(t *testing.T) {
	c := New("test")
	var normal, once, raw, subscriptions, presenceSnapshots atomic.Int32
	off := c.Listen(func(*MessageCreate) { normal.Add(1) })
	Once(c, func(*MessageCreate) { once.Add(1) })
	c.OnRaw(func(e *RawEvent) {
		if e.Name == "MESSAGE_CREATE" {
			raw.Add(1)
		}
	})
	c.On(func(*SubscriptionCreate) { subscriptions.Add(1) })
	c.On(func(e *PresencesReplace) { presenceSnapshots.Add(int32(len(*e))) })

	payload := []byte(`{"id":"1","channel_id":"2","content":"hi","type":0}`)
	c.dispatch("MESSAGE_CREATE", payload)
	off()
	c.dispatch("MESSAGE_CREATE", payload)
	c.dispatch("SUBSCRIPTION_CREATE", []byte(`{"id":"3","user_id":"4","status":0}`))
	c.dispatch("PRESENCES_REPLACE", []byte(`[{"user":{"id":"5"},"guild_id":"6","status":"online"}]`))

	if normal.Load() != 1 || once.Load() != 1 || raw.Load() != 2 || subscriptions.Load() != 1 || presenceSnapshots.Load() != 1 {
		t.Fatalf("normal=%d once=%d raw=%d subscriptions=%d presenceSnapshots=%d",
			normal.Load(), once.Load(), raw.Load(), subscriptions.Load(), presenceSnapshots.Load())
	}
}

func TestInteractionAttachmentMetadataIsNested(t *testing.T) {
	response := &InteractionResponse{
		Type: CallbackChannelMessageWithSource,
		Data: &InteractionResponseData{Content: "cat"},
	}
	raw, err := withAttachments(response, []File{{Name: "cat.png", Description: "a cat"}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["attachments"]; exists {
		t.Fatal("interaction attachment metadata was written at the top level")
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", payload["data"])
	}
	attachments, ok := data["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("attachments = %#v", data["attachments"])
	}
}

func TestMonetisationTypes(t *testing.T) {
	sku := SKU{Type: SKUSubscription, Flags: SKUAvailable | SKUUserSubscription}
	if sku.Type != SKUSubscription || !sku.Flags.Has(SKUAvailable|SKUUserSubscription) {
		t.Fatalf("SKU helpers failed: %#v", sku)
	}
	if sku.Flags.Has(SKUGuildSubscription) {
		t.Fatal("SKU flags reported a flag that is not set")
	}

	raw, err := json.Marshal(Subscription{Status: SubscriptionEnding})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"status":2`) {
		t.Fatalf("subscription status JSON = %s", raw)
	}
}

func TestRESTRateLimitLifecycleEvent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"slow down","retry_after":0.001,"global":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := New("test")
	var event atomic.Int32
	c.On(func(e *RateLimit) {
		if e.Global && e.Wait() == time.Millisecond {
			event.Add(1)
		}
	})
	_, err := c.RequestRaw(context.Background(), RESTRequest{
		Method: http.MethodGet, Base: server.URL, Path: "/limited", NoAuth: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Load() != 1 {
		t.Fatalf("rate limit events = %d", event.Load())
	}
}

func TestRawRESTEscapeHatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q with NoAuth", got)
		}
		if got := r.Header.Get("X-Custom"); got != "yes" {
			t.Errorf("X-Custom = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "raw body" {
			t.Errorf("body = %q", body)
		}
		_, _ = w.Write([]byte("raw response"))
	}))
	defer server.Close()

	c := New("test")
	out, err := c.RequestRaw(context.Background(), RESTRequest{
		Method:      http.MethodPost,
		Base:        server.URL,
		Path:        "/anything",
		NoAuth:      true,
		RawBody:     []byte("raw body"),
		ContentType: "text/plain",
		Headers:     http.Header{"X-Custom": {"yes"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "raw response" {
		t.Fatalf("response = %q", out)
	}
}
