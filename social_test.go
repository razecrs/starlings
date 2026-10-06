package starlings

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type socialRoundTripFunc func(*http.Request) (*http.Response, error)

func (f socialRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSocialAuthenticationAndPayloads(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	transport := socialRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		path := req.URL.Path
		mu.Lock()
		seen[req.Method+" "+path] = true
		mu.Unlock()

		status := http.StatusOK
		response := `{}`
		switch {
		case req.Method == http.MethodPost && path == "/api/v10/lobbies":
			if got := req.Header.Get("Authorization"); got != "Bot bot-token" {
				t.Errorf("create lobby auth = %q", got)
			}
			if !strings.Contains(string(body), `"mode":"ranked"`) {
				t.Errorf("create lobby body = %s", body)
			}
			response = `{"id":"10","application_id":"20","metadata":{},"members":[],"flags":0}`

		case req.Method == http.MethodPost && path == "/api/v10/lobbies/10/messages":
			if got := req.Header.Get("Authorization"); got != "Bearer player-token" {
				t.Errorf("lobby message auth = %q", got)
			}
			if !strings.Contains(string(body), `"content":"ready?"`) {
				t.Errorf("lobby message body = %s", body)
			}
			response = `{"id":"30","type":0,"content":"ready?","lobby_id":"10","channel_id":"10","author":{"id":"40","username":"player"},"flags":0}`

		case req.Method == http.MethodPost && path == "/api/v10/partner-sdk/token":
			if got := req.Header.Get("Authorization"); got != "" {
				t.Errorf("credential exchange leaked auth header %q", got)
			}
			if !strings.Contains(string(body), `"external_auth_type":"OIDC"`) {
				t.Errorf("credential exchange body = %s", body)
			}
			response = `{"token_type":"Bearer","access_token":"social","expires_in":3600,"scope":"sdk.social_layer","id_token":"id"}`

		case req.Method == http.MethodPost && path == "/api/v10/partner-sdk/token/bot":
			if got := req.Header.Get("Authorization"); got != "Bot bot-token" {
				t.Errorf("bot exchange auth = %q", got)
			}
			response = `{"token_type":"Bearer","access_token":"social","expires_in":3600,"scope":"sdk.social_layer","id_token":"id"}`

		default:
			status = http.StatusNotFound
			response = `{"message":"unexpected test route"}`
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(response)),
			Request:    req,
		}, nil
	})

	c := New("bot-token", WithHTTPClient(&http.Client{Transport: transport}), WithLogger(discardLogger()))
	ctx := context.Background()
	lobby, err := c.CreateLobby(ctx, LobbyCreate{Metadata: map[string]string{"mode": "ranked"}})
	if err != nil || lobby.ID != 10 {
		t.Fatalf("CreateLobby = %#v, %v", lobby, err)
	}
	message, err := c.Social("player-token").SendLobbyMessage(ctx, 10, "ready?")
	if err != nil || message.Content != "ready?" {
		t.Fatalf("SendLobbyMessage = %#v, %v", message, err)
	}
	token, err := c.ExchangeProvisionalToken(ctx, ProvisionalCredentials{
		ClientID: 20, ExternalAuthType: ExternalAuthOIDC, ExternalAuthToken: "external",
	})
	if err != nil || token.AccessToken != "social" {
		t.Fatalf("ExchangeProvisionalToken = %#v, %v", token, err)
	}
	if _, err := c.ExchangeBotProvisionalToken(ctx, "game-user", "Player", 0); err != nil {
		t.Fatal(err)
	}

	for _, route := range []string{
		"POST /api/v10/lobbies",
		"POST /api/v10/lobbies/10/messages",
		"POST /api/v10/partner-sdk/token",
		"POST /api/v10/partner-sdk/token/bot",
	} {
		if !seen[route] {
			t.Errorf("route %s was not called", route)
		}
	}
}

func TestLobbyClearAndValidation(t *testing.T) {
	body, err := lobbyEditBody(LobbyEdit{ClearMetadata: true, ClearOverrideEventWebhooksURL: true})
	if err != nil {
		t.Fatal(err)
	}
	if value, exists := body["metadata"]; !exists || value != nil {
		t.Fatalf("clear metadata = %#v, exists=%v", value, exists)
	}
	if value, exists := body["override_event_webhooks_url"]; !exists || value != nil {
		t.Fatalf("clear webhook URL = %#v, exists=%v", value, exists)
	}

	member, err := lobbyMemberBody(LobbyMemberInput{ID: 1, ClearAdditionalName: true, Remove: true}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if value, exists := member["additional_name"]; !exists || value != nil {
		t.Fatalf("clear additional name = %#v, exists=%v", value, exists)
	}
	if member["remove_member"] != true {
		t.Fatalf("remove_member = %#v", member["remove_member"])
	}

	if _, err := lobbyCreateBody(LobbyCreate{IdleTimeoutSeconds: 4}); err == nil {
		t.Fatal("idle timeout below Discord's minimum was accepted")
	}
	if err := validateModerationMetadata(map[string]string{
		"1": "x", "2": "x", "3": "x", "4": "x", "5": "x", "6": "x",
	}); err == nil {
		t.Fatal("six moderation metadata keys were accepted")
	}
}
