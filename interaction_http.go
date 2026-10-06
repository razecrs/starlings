package starlings

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const maxInteractionBody = 1 << 20
const maxInteractionClockSkew = 5 * time.Minute

const (
	maxInteractionReplayEntries = 256
	maxInteractionReplayBody    = 256 << 10
)

type interactionReplayEntry struct {
	done    chan struct{}
	created time.Time
	status  int
	header  http.Header
	body    []byte
}

type interactionReplayCache struct {
	mu      sync.Mutex
	entries map[Snowflake]*interactionReplayEntry
}

func newInteractionReplayCache() *interactionReplayCache {
	return &interactionReplayCache{entries: make(map[Snowflake]*interactionReplayEntry)}
}

func (c *interactionReplayCache) acquire(id Snowflake, now time.Time) (*interactionReplayEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if now.Sub(entry.created) > maxInteractionClockSkew {
			select {
			case <-entry.done:
				delete(c.entries, key)
			default:
			}
		}
	}
	if entry := c.entries[id]; entry != nil {
		return entry, false
	}
	if len(c.entries) >= maxInteractionReplayEntries {
		var oldestID Snowflake
		var oldest *interactionReplayEntry
		for key, entry := range c.entries {
			select {
			case <-entry.done:
				if oldest == nil || entry.created.Before(oldest.created) {
					oldestID, oldest = key, entry
				}
			default:
			}
		}
		if oldest == nil {
			return nil, true // All slots are active; execute without retaining it.
		}
		delete(c.entries, oldestID)
	}
	entry := &interactionReplayEntry{done: make(chan struct{}), created: now}
	c.entries[id] = entry
	return entry, true
}

func (c *interactionReplayCache) finish(entry *interactionReplayEntry, response *interactionResponseCapture) {
	if entry == nil {
		return
	}
	c.mu.Lock()
	if response.body.Len() <= maxInteractionReplayBody {
		entry.status = response.statusCode()
		entry.header = response.header.Clone()
		entry.body = append([]byte(nil), response.body.Bytes()...)
	} else {
		// Still suppress duplicate side effects, without retaining a large body.
		entry.status = http.StatusNoContent
	}
	close(entry.done)
	c.mu.Unlock()
}

type interactionResponseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newInteractionResponseCapture() *interactionResponseCapture {
	return &interactionResponseCapture{header: make(http.Header)}
}

func (w *interactionResponseCapture) Header() http.Header { return w.header }
func (w *interactionResponseCapture) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *interactionResponseCapture) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}
func (w *interactionResponseCapture) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func writeInteractionResponse(w http.ResponseWriter, header http.Header, status int, body []byte) {
	for key, values := range header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// InteractionHandler returns a normal net/http handler for Discord's
// Interactions Endpoint URL. It verifies every request and sends interactions
// through the same handlers registered with Slash and On.
//
// publicKey is the hex value shown on the application's General Information
// page. Construct the handler once, then mount it wherever you like:
//
//	h, err := bot.InteractionHandler(os.Getenv("DISCORD_PUBLIC_KEY"))
//	if err != nil { log.Fatal(err) }
//	http.Handle("/interactions", h)
func (c *Client) InteractionHandler(publicKey string) (http.Handler, error) {
	key, err := hex.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("starlings: Discord public key must be 64 hexadecimal characters")
	}
	replays := newInteractionReplayCache()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.serveInteractionHTTP(w, r, ed25519.PublicKey(key), replays)
	}), nil
}

// VerifyInteraction verifies Discord's Ed25519 request signature. Most bots
// can use InteractionHandler; this function is available for custom routers,
// middleware stacks, and servers that manage request bodies themselves.
func VerifyInteraction(publicKey ed25519.PublicKey, signatureHex, timestamp string, body []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize || timestamp == "" {
		return false
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}
	message := make([]byte, 0, len(timestamp)+len(body))
	message = append(message, timestamp...)
	message = append(message, body...)
	return ed25519.Verify(publicKey, message, signature)
}

func (c *Client) serveInteractionHTTP(w http.ResponseWriter, r *http.Request, publicKey ed25519.PublicKey, replays *interactionReplayCache) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInteractionBody))
	if err != nil {
		http.Error(w, "interaction body is too large", http.StatusRequestEntityTooLarge)
		return
	}
	timestamp := r.Header.Get("X-Signature-Timestamp")
	if !freshInteractionTimestamp(timestamp, time.Now()) || !VerifyInteraction(publicKey,
		r.Header.Get("X-Signature-Ed25519"), timestamp, body) {
		http.Error(w, "invalid request signature", http.StatusUnauthorized)
		return
	}

	var interaction InteractionCreate
	if err := json.Unmarshal(body, &interaction); err != nil {
		http.Error(w, "invalid interaction JSON", http.StatusBadRequest)
		return
	}
	if !interaction.ID.IsZero() {
		entry, owner := replays.acquire(interaction.ID, time.Now())
		if !owner {
			select {
			case <-entry.done:
				writeInteractionResponse(w, entry.header, entry.status, entry.body)
			case <-r.Context().Done():
				http.Error(w, "duplicate interaction wait cancelled", http.StatusRequestTimeout)
			}
			return
		}
		capture := newInteractionResponseCapture()
		c.executeInteractionHTTP(capture, r, &interaction)
		replays.finish(entry, capture)
		writeInteractionResponse(w, capture.header, capture.statusCode(), capture.body.Bytes())
		return
	}
	c.executeInteractionHTTP(w, r, &interaction)
}

func (c *Client) executeInteractionHTTP(w http.ResponseWriter, r *http.Request, interaction *InteractionCreate) {
	interaction.bind(c)
	interaction.respondHTTP = func(_ context.Context, response InteractionResponse, files []File) error {
		if len(files) > 0 {
			body, contentType, err := multipartBody(request{Body: response, Files: files})
			if err != nil {
				return err
			}
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			_, err = w.Write(body)
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.MarshalWrite(w, response); err != nil {
			return fmt.Errorf("starlings: writing interaction response: %w", err)
		}
		return nil
	}

	if interaction.Type == InteractionPing {
		if err := interaction.Respond(r.Context(), InteractionResponse{Type: CallbackPong}); err != nil {
			c.log.Error("starlings: answering interaction ping", "err", err)
		}
		return
	}

	c.dispatchHTTPInteraction(interaction)
	if atomic.LoadInt32(&interaction.answered) == 0 {
		w.WriteHeader(http.StatusNoContent)
	}
}

func freshInteractionTimestamp(value string, now time.Time) bool {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Sub(time.Unix(seconds, 0))
	return delta >= -maxInteractionClockSkew && delta <= maxInteractionClockSkew
}

func (c *Client) dispatchHTTPInteraction(interaction *InteractionCreate) {
	slot := c.slotFor("INTERACTION_CREATE")
	if slot == nil {
		return
	}
	for _, handler := range slot.handlers {
		handler.(func(*InteractionCreate))(interaction)
	}
}
