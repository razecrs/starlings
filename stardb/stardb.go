// Package stardb provides small, optional persistence adapters for Starlings
// bots. It has no dependency on the Discord client and is safe to use on its
// own.
package stardb

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound    = errors.New("stardb: key not found")
	ErrClosed      = errors.New("stardb: store closed")
	ErrInvalidKey  = errors.New("stardb: invalid key")
	ErrTooLarge    = errors.New("stardb: size limit exceeded")
	ErrConflict    = errors.New("stardb: concurrent update conflict")
	ErrInsecureURL = errors.New("stardb: remote URL must use HTTPS")
)

// Store is the common API implemented by every StarDB backend. Values are
// encoded as JSON, so structs, maps, slices, and scalar values all work.
type Store interface {
	Get(context.Context, string, any) error
	Put(context.Context, string, any) error
	Delete(context.Context, string) error
	Keys(context.Context) ([]string, error)
	Close() error
}

// Load retrieves a value without requiring a temporary declaration.
func Load[T any](ctx context.Context, store Store, key string) (T, error) {
	var value T
	if store == nil {
		return value, fmt.Errorf("stardb: nil store")
	}
	if err := store.Get(ctx, key, &value); err != nil {
		return value, err
	}
	return value, nil
}

// Save is the typed-friendly spelling of Store.Put.
func Save[T any](ctx context.Context, store Store, key string, value T) error {
	if store == nil {
		return fmt.Errorf("stardb: nil store")
	}
	return store.Put(ctx, key, value)
}

// Limits bound attacker-controlled keys, values, local files, and remote
// responses. Zero fields receive conservative defaults.
type Limits struct {
	MaxKeyBytes      int
	MaxValueBytes    int64
	MaxFileBytes     int64
	MaxResponseBytes int64
}

type config struct {
	limits     Limits
	logger     *slog.Logger
	key        []byte
	httpClient *http.Client
	ownSQL     bool
}

// Option applies shared security and observability settings to a backend.
type Option func(*config) error

// WithLimits replaces one or more default limits. Negative values are
// rejected; zero leaves that field at its default.
func WithLimits(limits Limits) Option {
	return func(c *config) error {
		if limits.MaxKeyBytes < 0 || limits.MaxValueBytes < 0 || limits.MaxFileBytes < 0 || limits.MaxResponseBytes < 0 {
			return fmt.Errorf("stardb: limits cannot be negative")
		}
		if limits.MaxKeyBytes > 0 {
			c.limits.MaxKeyBytes = limits.MaxKeyBytes
		}
		if limits.MaxValueBytes > 0 {
			c.limits.MaxValueBytes = limits.MaxValueBytes
		}
		if limits.MaxFileBytes > 0 {
			c.limits.MaxFileBytes = limits.MaxFileBytes
		}
		if limits.MaxResponseBytes > 0 {
			c.limits.MaxResponseBytes = limits.MaxResponseBytes
		}
		return nil
	}
}

// WithLogger enables structured operational logs. Starlog integrates directly:
//
//	store, err := stardb.OpenJSON("bot.json", stardb.WithLogger(logs.Logger()))
//
// Keys are represented by short hashes and values and credentials are never
// logged.
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) error {
		c.logger = logger
		return nil
	}
}

// WithEncryption enables AES-256-GCM value encryption at rest. The key is
// copied and is overwritten when the store closes. Applications should load it
// from a secret manager or environment variable, never source control.
func WithEncryption(key []byte) Option {
	return func(c *config) error {
		if len(key) != 32 {
			return fmt.Errorf("stardb: encryption key must be exactly 32 bytes")
		}
		c.key = append([]byte(nil), key...)
		return nil
	}
}

// WithHTTPClient supplies the bounded client used by remote stores. A nil
// client is rejected. Callers remain responsible for closing custom transports.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) error {
		if client == nil {
			return fmt.Errorf("stardb: nil HTTP client")
		}
		c.httpClient = client
		return nil
	}
}

// WithOwnedSQLConnection makes SQLStore.Close close the supplied *sql.DB.
// Without it, the caller retains ownership, matching normal database/sql use.
func WithOwnedSQLConnection() Option {
	return func(c *config) error {
		c.ownSQL = true
		return nil
	}
}

func newConfig(options []Option) (config, error) {
	c := config{
		limits: Limits{
			MaxKeyBytes:      512,
			MaxValueBytes:    4 << 20,
			MaxFileBytes:     64 << 20,
			MaxResponseBytes: 8 << 20,
		},
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&c); err != nil {
			zero(c.key)
			return config{}, err
		}
	}
	return c, nil
}

func (c *config) validateKey(key string) error {
	if key == "" || !utf8.ValidString(key) || len(key) > c.limits.MaxKeyBytes {
		return ErrInvalidKey
	}
	for _, r := range key {
		if r == 0 || unicode.IsControl(r) {
			return ErrInvalidKey
		}
	}
	return nil
}

type blob struct {
	Version    int             `json:"v"`
	JSON       json.RawMessage `json:"json,omitempty"`
	Ciphertext []byte          `json:"ciphertext,omitempty"`
	Nonce      []byte          `json:"nonce,omitempty"`
}

func (c *config) encode(key string, value any) (blob, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return blob{}, fmt.Errorf("stardb: encode value: %w", err)
	}
	if int64(len(raw)) > c.limits.MaxValueBytes {
		return blob{}, ErrTooLarge
	}
	result := blob{Version: 1}
	if len(c.key) == 0 {
		result.JSON = raw
		return result, nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return blob{}, fmt.Errorf("stardb: initialize encryption: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return blob{}, fmt.Errorf("stardb: initialize authenticated encryption: %w", err)
	}
	result.Nonce = make([]byte, aead.NonceSize())
	if _, err := rand.Read(result.Nonce); err != nil {
		return blob{}, fmt.Errorf("stardb: generate nonce: %w", err)
	}
	result.Ciphertext = aead.Seal(nil, result.Nonce, raw, []byte(key))
	return result, nil
}

func (c *config) decode(key string, value blob, destination any) error {
	if destination == nil {
		return fmt.Errorf("stardb: nil destination")
	}
	if value.Version != 1 {
		return fmt.Errorf("stardb: unsupported record version %d", value.Version)
	}
	raw := []byte(value.JSON)
	if len(value.Ciphertext) != 0 || len(value.Nonce) != 0 {
		if len(c.key) == 0 {
			return fmt.Errorf("stardb: encrypted value requires an encryption key")
		}
		block, err := aes.NewCipher(c.key)
		if err != nil {
			return fmt.Errorf("stardb: initialize decryption: %w", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return fmt.Errorf("stardb: initialize authenticated decryption: %w", err)
		}
		if len(value.Nonce) != aead.NonceSize() {
			return fmt.Errorf("stardb: invalid encrypted record")
		}
		raw, err = aead.Open(nil, value.Nonce, value.Ciphertext, []byte(key))
		if err != nil {
			return fmt.Errorf("stardb: authenticate encrypted record: %w", err)
		}
	}
	if int64(len(raw)) > c.limits.MaxValueBytes {
		return ErrTooLarge
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return fmt.Errorf("stardb: decode value: %w", err)
	}
	return nil
}

func (c *config) log(ctx context.Context, level slog.Level, message, backend, key string, attrs ...any) {
	if c.logger == nil {
		return
	}
	base := []any{"backend", backend}
	if key != "" {
		base = append(base, "key_hash", keyHash(key))
	}
	c.logger.Log(ctx, level, message, append(base, attrs...)...)
}

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:6])
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func cloneBlob(value blob) blob {
	value.JSON = append(json.RawMessage(nil), value.JSON...)
	value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	value.Nonce = append([]byte(nil), value.Nonce...)
	return value
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for i, r := range value {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return !strings.HasPrefix(value, "_") || len(value) > 1
}
