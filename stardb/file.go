package stardb

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

type fileFormat uint8

const (
	formatJSON fileFormat = iota
	formatCSV
)

// FileStore is a concurrent, in-process key/value store persisted with atomic
// file replacement. A single file must not be opened by multiple processes.
type FileStore struct {
	mu     sync.RWMutex
	path   string
	format fileFormat
	cfg    config
	data   map[string]blob
	closed bool
}

// OpenJSON opens or creates an atomic JSON store. New files are owner-only.
func OpenJSON(path string, options ...Option) (*FileStore, error) {
	return openFile(path, formatJSON, options)
}

// OpenCSV opens or creates an RFC 4180 CSV store. JSON values are placed in a
// quoted field, so commas and newlines remain lossless.
func OpenCSV(path string, options ...Option) (*FileStore, error) {
	return openFile(path, formatCSV, options)
}

func openFile(path string, format fileFormat, options []Option) (*FileStore, error) {
	if path == "" {
		return nil, fmt.Errorf("stardb: empty file path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("stardb: resolve file path: %w", err)
	}
	cfg, err := newConfig(options)
	if err != nil {
		return nil, err
	}
	store := &FileStore{path: abs, format: format, cfg: cfg, data: make(map[string]blob)}
	if err := store.load(); err != nil {
		zero(store.cfg.key)
		return nil, err
	}
	store.cfg.log(context.Background(), slog.LevelDebug, "stardb opened", store.backend(), "")
	return store, nil
}

func (s *FileStore) Get(ctx context.Context, key string, destination any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.cfg.validateKey(key); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	value, ok := s.data[key]
	value = cloneBlob(value)
	if !ok {
		return ErrNotFound
	}
	if err := s.cfg.decode(key, value, destination); err != nil {
		return err
	}
	s.cfg.log(ctx, slog.LevelDebug, "stardb get", s.backend(), key)
	return nil
}

func (s *FileStore) Put(ctx context.Context, key string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.cfg.validateKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	encoded, err := s.cfg.encode(key, value)
	if err != nil {
		return err
	}
	next := cloneRecords(s.data)
	next[key] = encoded
	if err := s.persist(next); err != nil {
		return err
	}
	s.data = next
	s.cfg.log(ctx, slog.LevelDebug, "stardb put", s.backend(), key)
	return nil
}

func (s *FileStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.cfg.validateKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if _, ok := s.data[key]; !ok {
		return ErrNotFound
	}
	next := cloneRecords(s.data)
	delete(next, key)
	if err := s.persist(next); err != nil {
		return err
	}
	s.data = next
	s.cfg.log(ctx, slog.LevelDebug, "stardb delete", s.backend(), key)
	return nil
}

func (s *FileStore) Keys(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrClosed
	}
	keys := make([]string, 0, len(s.data))
	for key := range s.data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	zero(s.cfg.key)
	s.cfg.key = nil
	return nil
}

func (s *FileStore) backend() string {
	if s.format == formatCSV {
		return "csv"
	}
	return "json"
}

func (s *FileStore) load() error {
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stardb: inspect file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("stardb: database path must be a regular file")
	}
	if info.Size() > s.cfg.limits.MaxFileBytes {
		return ErrTooLarge
	}
	file, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("stardb: open file: %w", err)
	}
	defer file.Close()
	limited := io.LimitReader(file, s.cfg.limits.MaxFileBytes+1)
	if s.format == formatCSV {
		err = s.decodeCSV(limited)
	} else {
		err = s.decodeJSON(limited)
	}
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(s.path, 0o600); err != nil {
			return fmt.Errorf("stardb: secure file permissions: %w", err)
		}
	}
	return nil
}

type jsonFile struct {
	Version int             `json:"version"`
	Records map[string]blob `json:"records"`
}

func (s *FileStore) decodeJSON(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var file jsonFile
	if err := decoder.Decode(&file); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("stardb: decode JSON store: %w", err)
	}
	if file.Version != 1 {
		return fmt.Errorf("stardb: unsupported file version %d", file.Version)
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}
	return s.acceptRecords(file.Records)
}

func (s *FileStore) decodeCSV(reader io.Reader) error {
	rows := csv.NewReader(reader)
	rows.FieldsPerRecord = 5
	header, err := rows.Read()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stardb: decode CSV header: %w", err)
	}
	want := []string{"key", "version", "json", "ciphertext", "nonce"}
	for i := range want {
		if header[i] != want[i] {
			return fmt.Errorf("stardb: invalid CSV header")
		}
	}
	for {
		row, err := rows.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("stardb: decode CSV row: %w", err)
		}
		if _, exists := s.data[row[0]]; exists {
			return fmt.Errorf("stardb: duplicate CSV key")
		}
		version, err := strconv.Atoi(row[1])
		if err != nil {
			return fmt.Errorf("stardb: invalid CSV record version")
		}
		value := blob{Version: version, JSON: json.RawMessage(row[2])}
		if row[3] != "" {
			if err := json.Unmarshal([]byte(strconv.Quote(row[3])), &value.Ciphertext); err != nil {
				return fmt.Errorf("stardb: decode CSV ciphertext: %w", err)
			}
		}
		if row[4] != "" {
			if err := json.Unmarshal([]byte(strconv.Quote(row[4])), &value.Nonce); err != nil {
				return fmt.Errorf("stardb: decode CSV nonce: %w", err)
			}
		}
		s.data[row[0]] = value
	}
	return s.acceptRecords(s.data)
}

func (s *FileStore) acceptRecords(records map[string]blob) error {
	if records == nil {
		records = make(map[string]blob)
	}
	for key, value := range records {
		if err := s.cfg.validateKey(key); err != nil {
			return fmt.Errorf("stardb: invalid stored key: %w", err)
		}
		if value.Version != 1 {
			return fmt.Errorf("stardb: unsupported record version %d", value.Version)
		}
		if int64(len(value.JSON)+len(value.Ciphertext)) > s.cfg.limits.MaxValueBytes+64 {
			return ErrTooLarge
		}
	}
	s.data = records
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("stardb: trailing JSON value")
		}
		return fmt.Errorf("stardb: trailing JSON: %w", err)
	}
	return nil
}

func (s *FileStore) persist(records map[string]blob) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("stardb: create database directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".stardb-*")
	if err != nil {
		return fmt.Errorf("stardb: create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("stardb: secure temporary file: %w", err)
	}
	counting := &limitWriter{writer: temporary, remaining: s.cfg.limits.MaxFileBytes}
	if s.format == formatCSV {
		err = encodeCSV(counting, records)
	} else {
		encoder := json.NewEncoder(counting)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(jsonFile{Version: 1, Records: records})
	}
	if err != nil {
		return fmt.Errorf("stardb: encode store: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("stardb: sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("stardb: close temporary file: %w", err)
	}
	if err := replaceFile(temporaryName, s.path); err != nil {
		return fmt.Errorf("stardb: commit file: %w", err)
	}
	committed = true
	return syncDirectory(filepath.Dir(s.path))
}

func encodeCSV(writer io.Writer, records map[string]blob) error {
	rows := csv.NewWriter(writer)
	if err := rows.Write([]string{"key", "version", "json", "ciphertext", "nonce"}); err != nil {
		return err
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := records[key]
		ciphertext, _ := json.Marshal(value.Ciphertext)
		nonce, _ := json.Marshal(value.Nonce)
		var ciphertextText, nonceText string
		_ = json.Unmarshal(ciphertext, &ciphertextText)
		_ = json.Unmarshal(nonce, &nonceText)
		if err := rows.Write([]string{key, strconv.Itoa(value.Version), string(value.JSON), ciphertextText, nonceText}); err != nil {
			return err
		}
	}
	rows.Flush()
	return rows.Error()
}

type limitWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, ErrTooLarge
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func cloneRecords(records map[string]blob) map[string]blob {
	result := make(map[string]blob, len(records))
	for key, value := range records {
		result[key] = cloneBlob(value)
	}
	return result
}
