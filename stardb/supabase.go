package stardb

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"sync"
)

// SupabaseConfig configures the Supabase/PostgREST adapter. The table must
// contain a unique text key column and a json/jsonb value column.
type SupabaseConfig struct {
	ProjectURL  string
	APIKey      string
	BearerToken string
	Table       string
	KeyColumn   string
	ValueColumn string
}

type SupabaseStore struct {
	mu          sync.RWMutex
	base        *url.URL
	table       string
	keyColumn   string
	valueColumn string
	apiKey      []byte
	bearer      []byte
	client      *http.Client
	cfg         config
	closed      bool
}

func OpenSupabase(settings SupabaseConfig, options ...Option) (*SupabaseStore, error) {
	base, err := secureBaseURL(settings.ProjectURL)
	if err != nil {
		return nil, err
	}
	if settings.APIKey == "" {
		return nil, fmt.Errorf("stardb: Supabase API key is required")
	}
	if settings.BearerToken == "" {
		settings.BearerToken = settings.APIKey
	}
	if settings.KeyColumn == "" {
		settings.KeyColumn = "key"
	}
	if settings.ValueColumn == "" {
		settings.ValueColumn = "value"
	}
	if !validIdentifier(settings.Table) || !validIdentifier(settings.KeyColumn) || !validIdentifier(settings.ValueColumn) {
		return nil, fmt.Errorf("stardb: invalid Supabase identifier")
	}
	cfg, err := newConfig(options)
	if err != nil {
		return nil, err
	}
	return &SupabaseStore{
		base: base, table: settings.Table, keyColumn: settings.KeyColumn, valueColumn: settings.ValueColumn,
		apiKey: []byte(settings.APIKey), bearer: []byte(settings.BearerToken),
		client: safeHTTPClient(cfg.httpClient), cfg: cfg,
	}, nil
}

type supabaseRow struct {
	Key   string `json:"-"`
	Value blob   `json:"-"`
}

func (s *SupabaseStore) Get(ctx context.Context, key string, destination any) error {
	if err := s.lockKey(ctx, key); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	var rows []map[string]any
	_, err := remoteRequest(ctx, s.client, http.MethodGet, s.endpoint(url.Values{
		"select":    {s.keyColumn + "," + s.valueColumn},
		s.keyColumn: {"eq." + key},
		"limit":     {"1"},
	}), s.headers(), nil, s.cfg.limits.MaxResponseBytes, &rows, "supabase", http.StatusOK)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	record, err := mapBlob(rows[0][s.valueColumn])
	if err != nil {
		return err
	}
	if err := s.cfg.decode(key, record, destination); err != nil {
		return err
	}
	s.cfg.log(ctx, slog.LevelDebug, "stardb get", "supabase", key)
	return nil
}

func (s *SupabaseStore) Put(ctx context.Context, key string, source any) error {
	if err := s.lockKey(ctx, key); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	record, err := s.cfg.encode(key, source)
	if err != nil {
		return err
	}
	body := map[string]any{s.keyColumn: key, s.valueColumn: record}
	headers := s.headers()
	headers.Set("Prefer", "resolution=merge-duplicates,return=minimal")
	_, err = remoteRequest(ctx, s.client, http.MethodPost, s.endpoint(url.Values{"on_conflict": {s.keyColumn}}), headers, body, s.cfg.limits.MaxResponseBytes, nil, "supabase", http.StatusCreated, http.StatusOK, http.StatusNoContent)
	if err == nil {
		s.cfg.log(ctx, slog.LevelDebug, "stardb put", "supabase", key)
	}
	return err
}

func (s *SupabaseStore) Delete(ctx context.Context, key string) error {
	if err := s.lockKey(ctx, key); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	headers := s.headers()
	headers.Set("Prefer", "return=representation")
	var rows []map[string]any
	_, err := remoteRequest(ctx, s.client, http.MethodDelete, s.endpoint(url.Values{
		"select":    {s.keyColumn},
		s.keyColumn: {"eq." + key},
	}), headers, nil, s.cfg.limits.MaxResponseBytes, &rows, "supabase", http.StatusOK)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	s.cfg.log(ctx, slog.LevelDebug, "stardb delete", "supabase", key)
	return nil
}

func (s *SupabaseStore) Keys(ctx context.Context) ([]string, error) {
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.mu.RUnlock()
	var rows []map[string]any
	_, err := remoteRequest(ctx, s.client, http.MethodGet, s.endpoint(url.Values{
		"select": {s.keyColumn},
		"order":  {s.keyColumn + ".asc"},
	}), s.headers(), nil, s.cfg.limits.MaxResponseBytes, &rows, "supabase", http.StatusOK)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		key, ok := row[s.keyColumn].(string)
		if !ok || s.cfg.validateKey(key) != nil {
			return nil, fmt.Errorf("stardb: invalid Supabase key in response")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *SupabaseStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	zero(s.apiKey)
	zero(s.bearer)
	zero(s.cfg.key)
	s.apiKey, s.bearer, s.cfg.key = nil, nil, nil
	return nil
}

func (s *SupabaseStore) lockKey(ctx context.Context, key string) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	if err := s.cfg.validateKey(key); err != nil {
		s.mu.RUnlock()
		return err
	}
	if !supabaseKey(key) {
		s.mu.RUnlock()
		return ErrInvalidKey
	}
	return nil
}

func (s *SupabaseStore) lock(ctx context.Context) error {
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

func (s *SupabaseStore) endpoint(query url.Values) *url.URL {
	copy := *s.base
	copy.Path += "/rest/v1/" + s.table
	copy.RawQuery = query.Encode()
	return &copy
}

func (s *SupabaseStore) headers() http.Header {
	headers := make(http.Header)
	headers.Set("apikey", string(s.apiKey))
	headers.Set("Authorization", "Bearer "+string(s.bearer))
	return headers
}

func mapBlob(value any) (blob, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return blob{}, fmt.Errorf("stardb: encode Supabase record: %w", err)
	}
	var result blob
	if err := json.Unmarshal(raw, &result); err != nil {
		return blob{}, fmt.Errorf("stardb: decode Supabase record: %w", err)
	}
	return result, nil
}

// PostgREST filter values have their own grammar after URL decoding. Keeping
// keys to an unambiguous identifier alphabet prevents filter-expression
// injection while retaining common guild:user and namespaced keys.
func supabaseKey(value string) bool {
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '_', '.', '~', ':':
			continue
		default:
			return false
		}
	}
	return true
}
