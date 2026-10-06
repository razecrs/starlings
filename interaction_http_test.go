package starlings

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func signedInteractionRequest(t *testing.T, privateKey ed25519.PrivateKey, body string) *http.Request {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	return signedInteractionRequestAt(t, privateKey, body, timestamp)
}

func signedInteractionRequestAt(t *testing.T, privateKey ed25519.PrivateKey, body, timestamp string) *http.Request {
	t.Helper()
	signature := ed25519.Sign(privateKey, append([]byte(timestamp), body...))
	r := httptest.NewRequest(http.MethodPost, "/interactions", strings.NewReader(body))
	r.Header.Set("X-Signature-Timestamp", timestamp)
	r.Header.Set("X-Signature-Ed25519", hex.EncodeToString(signature))
	return r
}

func TestInteractionHandlerRejectsStaleSignedReplay(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h, err := testClient().InteractionHandler(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"1","application_id":"2","type":1,"token":"x","version":1}`
	timestamp := strconv.FormatInt(time.Now().Add(-maxInteractionClockSkew-time.Second).Unix(), 10)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedInteractionRequestAt(t, privateKey, body, timestamp))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("stale signed request status = %d, want 401", w.Code)
	}
}

func TestInteractionHandlerPing(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := testClient()
	h, err := c.InteractionHandler(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedInteractionRequest(t, privateKey, `{"id":"1","application_id":"2","type":1,"token":"x","version":1}`))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"type":1`) {
		t.Fatalf("ping response: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestInteractionHandlerRoutesSlashReply(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := testClient()
	c.Slash("ping", "check", func(i *InteractionCreate) {
		if err := i.Reply("pong"); err != nil {
			t.Errorf("Reply: %v", err)
		}
	})
	h, err := c.InteractionHandler(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"1","application_id":"2","type":2,"data":{"id":"3","name":"ping","type":1},"token":"x","version":1}`
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedInteractionRequest(t, privateKey, body))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"content":"pong"`) {
		t.Fatalf("slash response: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestInteractionHandlerRejectsBadSignature(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := testClient()
	h, err := c.InteractionHandler(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/interactions", strings.NewReader(`{"type":1}`))
	r.Header.Set("X-Signature-Timestamp", "1700000000")
	r.Header.Set("X-Signature-Ed25519", strings.Repeat("00", ed25519.SignatureSize))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestInteractionHandlerRejectsBadPublicKey(t *testing.T) {
	if _, err := testClient().InteractionHandler("not-a-key"); err == nil {
		t.Fatal("invalid public key was accepted")
	}
}

func TestInteractionHandlerReplaysResponseWithoutRedispatch(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := testClient()
	var calls atomic.Int32
	c.Slash("once", "deduplicated", func(i *InteractionCreate) {
		calls.Add(1)
		if err := i.Reply("same response"); err != nil {
			t.Errorf("Reply: %v", err)
		}
	})
	h, err := c.InteractionHandler(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"77","application_id":"2","type":2,"data":{"id":"3","name":"once","type":1},"token":"x","version":1}`
	responses := make([]*httptest.ResponseRecorder, 2)
	for i := range responses {
		responses[i] = httptest.NewRecorder()
		h.ServeHTTP(responses[i], signedInteractionRequest(t, privateKey, body))
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
	if responses[0].Code != responses[1].Code || responses[0].Body.String() != responses[1].Body.String() {
		t.Fatalf("replayed response differs: first=%d %q second=%d %q",
			responses[0].Code, responses[0].Body.String(), responses[1].Code, responses[1].Body.String())
	}
}

func TestInteractionHandlerCoalescesConcurrentDuplicates(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := testClient()
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	c.Slash("once", "deduplicated", func(i *InteractionCreate) {
		calls.Add(1)
		close(entered)
		<-release
		_ = i.Reply("done")
	})
	h, err := c.InteractionHandler(hex.EncodeToString(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"88","application_id":"2","type":2,"data":{"id":"3","name":"once","type":1},"token":"x","version":1}`
	responses := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		h.ServeHTTP(responses[0], signedInteractionRequest(t, privateKey, body))
	}()
	<-entered
	go func() {
		defer wg.Done()
		h.ServeHTTP(responses[1], signedInteractionRequest(t, privateKey, body))
	}()
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
	if responses[0].Body.String() != responses[1].Body.String() {
		t.Fatalf("concurrent response mismatch: %q != %q", responses[0].Body.String(), responses[1].Body.String())
	}
}
