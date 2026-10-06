package stardb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFirebaseEndToEnd(t *testing.T) {
	const token = "firebase-secret-token"
	var mu sync.Mutex
	records := map[string]blob{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("missing or wrong authorization header")
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.Contains(request.URL.RawQuery, token) || strings.Contains(request.RequestURI, token) {
			t.Error("credential leaked into Firebase URL")
		}
		if request.URL.Path == "/bot.json" {
			if request.URL.Query().Get("shallow") != "true" {
				t.Error("keys request was not shallow")
			}
			mu.Lock()
			defer mu.Unlock()
			result := make(map[string]bool, len(records))
			for key := range records {
				result[key] = true
			}
			_ = json.NewEncoder(response).Encode(result)
			return
		}
		path := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/bot/"), ".json")
		mu.Lock()
		defer mu.Unlock()
		switch request.Method {
		case http.MethodPut:
			var value blob
			if err := json.NewDecoder(request.Body).Decode(&value); err != nil {
				t.Error(err)
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			records[path] = value
			_ = json.NewEncoder(response).Encode(value)
		case http.MethodGet:
			value, ok := records[path]
			if request.Header.Get("X-Firebase-ETag") == "true" && ok {
				response.Header().Set("ETag", `"v1"`)
			}
			if !ok {
				_, _ = io.WriteString(response, "null")
				return
			}
			_ = json.NewEncoder(response).Encode(value)
		case http.MethodDelete:
			if request.Header.Get("If-Match") != `"v1"` {
				response.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			delete(records, path)
			_, _ = io.WriteString(response, "null")
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	store, err := OpenFirebase(FirebaseConfig{DatabaseURL: server.URL, Token: token, Prefix: "bot"}, WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	want := testValue{Name: "Luma", Count: 3}
	if err := store.Put(context.Background(), "guild-1", want); err != nil {
		t.Fatal(err)
	}
	var got testValue
	if err := store.Get(context.Background(), "guild-1", &got); err != nil || got.Name != want.Name {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	keys, err := store.Keys(context.Background())
	if err != nil || strings.Join(keys, ",") != "guild-1" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	if err := store.Delete(context.Background(), "guild-1"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(store.Get(context.Background(), "guild-1", &got), ErrNotFound) {
		t.Fatal("deleted Firebase key was found")
	}
	if !errors.Is(store.Delete(context.Background(), "guild-1"), ErrNotFound) {
		t.Fatal("deleting missing Firebase key should return ErrNotFound")
	}
}

func TestSupabaseEndToEnd(t *testing.T) {
	const apiKey = "supabase-secret-api-key"
	var mu sync.Mutex
	records := map[string]blob{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("apikey") != apiKey || request.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Errorf("Supabase credentials missing from headers")
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		if request.URL.Path != "/rest/v1/star_data" {
			t.Errorf("unexpected Supabase path %q", request.URL.Path)
		}
		if strings.Contains(request.RequestURI, apiKey) {
			t.Error("credential leaked into Supabase URL")
		}
		mu.Lock()
		defer mu.Unlock()
		switch request.Method {
		case http.MethodPost:
			var row map[string]json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&row); err != nil {
				t.Error(err)
				return
			}
			var key string
			var value blob
			if err := json.Unmarshal(row["key"], &key); err != nil {
				t.Error(err)
			}
			if err := json.Unmarshal(row["value"], &value); err != nil {
				t.Error(err)
			}
			records[key] = value
			response.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			filter := request.URL.Query().Get("key")
			if filter != "" {
				key := strings.TrimPrefix(filter, "eq.")
				if value, ok := records[key]; ok {
					_ = json.NewEncoder(response).Encode([]any{map[string]any{"key": key, "value": value}})
				} else {
					_, _ = io.WriteString(response, "[]")
				}
				return
			}
			keys := make([]string, 0, len(records))
			for key := range records {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			rows := make([]map[string]any, 0, len(keys))
			for _, key := range keys {
				rows = append(rows, map[string]any{"key": key})
			}
			_ = json.NewEncoder(response).Encode(rows)
		case http.MethodDelete:
			key := strings.TrimPrefix(request.URL.Query().Get("key"), "eq.")
			if _, ok := records[key]; !ok {
				_, _ = io.WriteString(response, "[]")
				return
			}
			delete(records, key)
			_ = json.NewEncoder(response).Encode([]any{map[string]any{"key": key}})
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	store, err := OpenSupabase(SupabaseConfig{ProjectURL: server.URL, APIKey: apiKey, Table: "star_data"}, WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Put(context.Background(), "one", testValue{Name: "Comet", Count: 4}); err != nil {
		t.Fatal(err)
	}
	var got testValue
	if err := store.Get(context.Background(), "one", &got); err != nil || got.Name != "Comet" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	keys, err := store.Keys(context.Background())
	if err != nil || strings.Join(keys, ",") != "one" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	if err := store.Delete(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(store.Delete(context.Background(), "one"), ErrNotFound) {
		t.Fatal("deleting missing Supabase key should return ErrNotFound")
	}
}

func TestRemoteSecurityControls(t *testing.T) {
	if _, err := OpenFirebase(FirebaseConfig{DatabaseURL: "http://example.com", Token: "secret"}); !errors.Is(err, ErrInsecureURL) {
		t.Fatalf("insecure Firebase URL: %v", err)
	}
	if _, err := OpenSupabase(SupabaseConfig{ProjectURL: "http://example.com", APIKey: "secret", Table: "data"}); !errors.Is(err, ErrInsecureURL) {
		t.Fatalf("insecure Supabase URL: %v", err)
	}
	if _, err := OpenSupabase(SupabaseConfig{ProjectURL: "https://example.com", APIKey: "secret", Table: "data; DROP TABLE data"}); err == nil {
		t.Fatal("SQL-like Supabase table identifier was accepted")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	store, err := OpenSupabase(SupabaseConfig{ProjectURL: server.URL, APIKey: "secret", Table: "data"}, WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Put(context.Background(), `x),or(key.neq.safe`, 1); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("PostgREST filter injection key was accepted: %v", err)
	}
}

func TestRemoteRefusesRedirectAndDoesNotLeakAuthorization(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		destinationCalls.Add(1)
	}))
	defer destination.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := source.Client()
	// Trust both test certificates; redirect must still be refused before TLS.
	client.Transport = &dualServerTransport{source: source.Client().Transport, destination: destination.Client().Transport}
	store, err := OpenFirebase(FirebaseConfig{DatabaseURL: source.URL, Token: "never-forward-me"}, WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var value any
	err = store.Get(context.Background(), "key", &value)
	var remote *RemoteError
	if !errors.As(err, &remote) || remote.Status != http.StatusTemporaryRedirect {
		t.Fatalf("expected safe redirect error, got %v", err)
	}
	if destinationCalls.Load() != 0 {
		t.Fatal("redirect destination was contacted")
	}
}

// dualServerTransport is only needed because each TLS test server creates its
// own certificate pool. Requests never reach the second transport in this test.
type dualServerTransport struct {
	source      http.RoundTripper
	destination http.RoundTripper
}

func (t *dualServerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	parsed, _ := url.Parse(request.URL.String())
	if strings.Contains(parsed.Host, "127.0.0.1") {
		return t.source.RoundTrip(request)
	}
	return t.destination.RoundTrip(request)
}

func TestRemoteResponseLimitAndErrorRedaction(t *testing.T) {
	const secret = "top-secret-token"
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "huge") {
			_, _ = fmt.Fprint(response, `"`+strings.Repeat("x", 2048)+`"`)
			return
		}
		response.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(response, "server accidentally echoed "+secret)
	}))
	defer server.Close()
	store, err := OpenFirebase(FirebaseConfig{DatabaseURL: server.URL, Token: secret}, WithHTTPClient(server.Client()), WithLimits(Limits{MaxResponseBytes: 128}))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var value any
	if err := store.Get(context.Background(), "huge", &value); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized response error=%v", err)
	}
	err = store.Get(context.Background(), "failure", &value)
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "echoed") {
		t.Fatalf("remote error leaked response content: %v", err)
	}
}
