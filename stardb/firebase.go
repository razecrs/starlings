package stardb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// FirebaseConfig configures Firebase Realtime Database REST access. Token is
// sent only in the Authorization header, never the URL.
type FirebaseConfig struct {
	DatabaseURL string
	Token       string
	Prefix      string
}

// FirebaseStore implements Store on Firebase Realtime Database.
type FirebaseStore struct {
	mu     sync.RWMutex
	base   *url.URL
	prefix string
	token  []byte
	client *http.Client
	cfg    config
	closed bool
}

func OpenFirebase(settings FirebaseConfig, options ...Option) (*FirebaseStore, error) {
	base, err := secureBaseURL(settings.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if settings.Token == "" {
		return nil, fmt.Errorf("stardb: Firebase token is required")
	}
	prefix := strings.Trim(settings.Prefix, "/")
	if prefix != "" && !firebaseKey(prefix) {
		return nil, fmt.Errorf("stardb: invalid Firebase prefix")
	}
	cfg, err := newConfig(options)
	if err != nil {
		return nil, err
	}
	return &FirebaseStore{
		base: base, prefix: prefix, token: []byte(settings.Token),
		client: safeHTTPClient(cfg.httpClient), cfg: cfg,
	}, nil
}

func (s *FirebaseStore) Get(ctx context.Context, key string, destination any) error {
	if err := s.lockKey(ctx, key); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	var value json.RawMessage
	_, err := remoteRequest(ctx, s.client, http.MethodGet, s.endpoint(key, false), s.headers(), nil, s.cfg.limits.MaxResponseBytes, &value, "firebase", http.StatusOK)
	if err != nil {
		return err
	}
	if string(value) == "null" {
		return ErrNotFound
	}
	var record blob
	if err := json.Unmarshal(value, &record); err != nil {
		return fmt.Errorf("stardb: decode Firebase record: %w", err)
	}
	if err := s.cfg.decode(key, record, destination); err != nil {
		return err
	}
	s.cfg.log(ctx, slog.LevelDebug, "stardb get", "firebase", key)
	return nil
}

func (s *FirebaseStore) Put(ctx context.Context, key string, source any) error {
	if err := s.lockKey(ctx, key); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	record, err := s.cfg.encode(key, source)
	if err != nil {
		return err
	}
	_, err = remoteRequest(ctx, s.client, http.MethodPut, s.endpoint(key, false), s.headers(), record, s.cfg.limits.MaxResponseBytes, nil, "firebase", http.StatusOK, http.StatusNoContent)
	if err == nil {
		s.cfg.log(ctx, slog.LevelDebug, "stardb put", "firebase", key)
	}
	return err
}

func (s *FirebaseStore) update(ctx context.Context, key string, destination any, reset func(bool) error, change func() error) error {
	if err := s.lockKey(ctx, key); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	const attempts = 8
	for attempt := 0; attempt < attempts; attempt++ {
		headers := s.headers()
		headers.Set("X-Firebase-ETag", "true")
		var current json.RawMessage
		response, err := remoteRequest(ctx, s.client, http.MethodGet, s.endpoint(key, false), headers, nil, s.cfg.limits.MaxResponseBytes, &current, "firebase", http.StatusOK)
		if err != nil {
			return err
		}
		etag := response.Header.Get("ETag")
		if etag == "" {
			return fmt.Errorf("stardb: Firebase did not return an ETag")
		}
		exists := strings.TrimSpace(string(current)) != "null"
		if err := reset(exists); err != nil {
			return err
		}
		if exists {
			var record blob
			if err := json.Unmarshal(current, &record); err != nil {
				return fmt.Errorf("stardb: decode Firebase record: %w", err)
			}
			if err := s.cfg.decode(key, record, destination); err != nil {
				return err
			}
		}
		if err := change(); err != nil {
			return err
		}
		record, err := s.cfg.encode(key, destination)
		if err != nil {
			return err
		}
		headers = s.headers()
		headers.Set("If-Match", etag)
		_, err = remoteRequest(ctx, s.client, http.MethodPut, s.endpoint(key, false), headers, record, s.cfg.limits.MaxResponseBytes, nil, "firebase", http.StatusOK, http.StatusNoContent)
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		s.cfg.log(ctx, slog.LevelDebug, "stardb update", "firebase", key)
		return nil
	}
	return ErrConflict
}

func (s *FirebaseStore) Delete(ctx context.Context, key string) error {
	if err := s.lockKey(ctx, key); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	headers := s.headers()
	headers.Set("X-Firebase-ETag", "true")
	var existing json.RawMessage
	response, err := remoteRequest(ctx, s.client, http.MethodGet, s.endpoint(key, false), headers, nil, s.cfg.limits.MaxResponseBytes, &existing, "firebase", http.StatusOK)
	if err != nil {
		return err
	}
	if string(existing) == "null" {
		return ErrNotFound
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		return fmt.Errorf("stardb: Firebase did not return an ETag")
	}
	headers = s.headers()
	headers.Set("If-Match", etag)
	_, err = remoteRequest(ctx, s.client, http.MethodDelete, s.endpoint(key, false), headers, nil, s.cfg.limits.MaxResponseBytes, nil, "firebase", http.StatusOK, http.StatusNoContent)
	if err == nil {
		s.cfg.log(ctx, slog.LevelDebug, "stardb delete", "firebase", key)
	}
	return err
}

func (s *FirebaseStore) Keys(ctx context.Context) ([]string, error) {
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.mu.RUnlock()
	var result map[string]bool
	_, err := remoteRequest(ctx, s.client, http.MethodGet, s.endpoint("", true), s.headers(), nil, s.cfg.limits.MaxResponseBytes, &result, "firebase", http.StatusOK)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(result))
	for key := range result {
		if err := s.cfg.validateKey(key); err != nil {
			return nil, fmt.Errorf("stardb: invalid Firebase key in response")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *FirebaseStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	zero(s.token)
	zero(s.cfg.key)
	s.token, s.cfg.key = nil, nil
	return nil
}

func (s *FirebaseStore) lockKey(ctx context.Context, key string) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	if err := s.cfg.validateKey(key); err != nil {
		s.mu.RUnlock()
		return err
	}
	if !firebaseKey(key) {
		s.mu.RUnlock()
		return ErrInvalidKey
	}
	return nil
}

func (s *FirebaseStore) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	return nil
}

func (s *FirebaseStore) endpoint(key string, shallow bool) *url.URL {
	copy := *s.base
	segments := make([]string, 0, 2)
	if s.prefix != "" {
		segments = append(segments, s.prefix)
	}
	if key != "" {
		segments = append(segments, key)
	}
	copy.Path = strings.TrimRight(copy.Path, "/") + "/" + strings.Join(segments, "/") + ".json"
	if shallow {
		copy.RawQuery = "shallow=true"
	}
	return &copy
}

func (s *FirebaseStore) headers() http.Header {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+string(s.token))
	return headers
}

func firebaseKey(value string) bool {
	return !strings.ContainsAny(value, ".#$[]/")
}
