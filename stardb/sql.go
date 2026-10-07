package stardb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

// SQLDialect selects placeholder, quoting, schema, and upsert syntax. StarDB
// deliberately uses database/sql rather than forcing a database driver.
type SQLDialect uint8

const (
	SQLSQLite SQLDialect = iota
	SQLPostgres
	SQLMySQL
)

// SQLError preserves an underlying driver error for errors.Is/errors.As while
// keeping its potentially value-bearing text out of normal logs.
type SQLError struct {
	Operation string
	cause     error
}

func (e *SQLError) Error() string { return "stardb: SQL " + e.Operation + " failed" }
func (e *SQLError) Unwrap() error { return e.cause }

func sqlFailure(operation string, err error) error {
	return &SQLError{Operation: operation, cause: err}
}

// SQLStore persists JSON values in a two-column key/value table. All values are
// query parameters; only a strictly validated table identifier enters SQL text.
type SQLStore struct {
	mu      sync.RWMutex
	db      *sql.DB
	table   string
	quoted  string
	dialect SQLDialect
	cfg     config
	closed  bool
}

// OpenSQL creates a driver-neutral SQL store. Call EnsureSchema explicitly if
// StarDB should create its table. OpenSQL never makes a network connection by
// itself and does not own db unless WithOwnedSQLConnection is supplied.
func OpenSQL(db *sql.DB, table string, dialect SQLDialect, options ...Option) (*SQLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("stardb: nil SQL database")
	}
	if !validIdentifier(table) {
		return nil, fmt.Errorf("stardb: invalid SQL table name")
	}
	if dialect > SQLMySQL {
		return nil, fmt.Errorf("stardb: unsupported SQL dialect")
	}
	cfg, err := newConfig(options)
	if err != nil {
		return nil, err
	}
	quoted := `"` + table + `"`
	if dialect == SQLMySQL {
		quoted = "`" + table + "`"
	}
	return &SQLStore{db: db, table: table, quoted: quoted, dialect: dialect, cfg: cfg}, nil
}

// EnsureSchema creates the key/value table when it is absent. Schema changes
// remain the application's responsibility.
func (s *SQLStore) EnsureSchema(ctx context.Context) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	var statement string
	switch s.dialect {
	case SQLPostgres:
		statement = "CREATE TABLE IF NOT EXISTS " + s.quoted + " (key TEXT PRIMARY KEY, value BYTEA NOT NULL)"
	case SQLMySQL:
		statement = "CREATE TABLE IF NOT EXISTS " + s.quoted + " (`key` VARBINARY(512) PRIMARY KEY, `value` LONGBLOB NOT NULL)"
	default:
		statement = "CREATE TABLE IF NOT EXISTS " + s.quoted + " (key TEXT PRIMARY KEY, value BLOB NOT NULL)"
	}
	if _, err := s.db.ExecContext(ctx, statement); err != nil {
		return sqlFailure("create schema", err)
	}
	s.cfg.log(ctx, slog.LevelInfo, "stardb schema ready", "sql", "", "dialect", s.dialect.String())
	return nil
}

func (s *SQLStore) Get(ctx context.Context, key string, destination any) error {
	if err := s.cfg.validateKey(key); err != nil {
		return err
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	keyColumn, valueColumn := "key", "value"
	if s.dialect == SQLMySQL {
		keyColumn, valueColumn = "`key`", "`value`"
	}
	query := "SELECT " + valueColumn + " FROM " + s.quoted + " WHERE " + keyColumn + " = " + s.placeholder(1)
	var packed []byte
	if err := s.db.QueryRowContext(ctx, query, key).Scan(&packed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return sqlFailure("get", err)
	}
	if int64(len(packed)) > s.cfg.limits.MaxValueBytes+1024 {
		return ErrTooLarge
	}
	var value blob
	if err := json.Unmarshal(packed, &value); err != nil {
		return fmt.Errorf("stardb: decode SQL record: %w", err)
	}
	if err := s.cfg.decode(key, value, destination); err != nil {
		return err
	}
	s.cfg.log(ctx, slog.LevelDebug, "stardb get", "sql", key)
	return nil
}

func (s *SQLStore) Put(ctx context.Context, key string, source any) error {
	if err := s.cfg.validateKey(key); err != nil {
		return err
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	value, err := s.cfg.encode(key, source)
	if err != nil {
		return err
	}
	packed, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("stardb: encode SQL record: %w", err)
	}
	var statement string
	switch s.dialect {
	case SQLPostgres:
		statement = "INSERT INTO " + s.quoted + " (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value"
	case SQLMySQL:
		statement = "INSERT INTO " + s.quoted + " (`key`, `value`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `value` = VALUES(`value`)"
	default:
		statement = "INSERT INTO " + s.quoted + " (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value"
	}
	if _, err := s.db.ExecContext(ctx, statement, key, packed); err != nil {
		return sqlFailure("put", err)
	}
	s.cfg.log(ctx, slog.LevelDebug, "stardb put", "sql", key)
	return nil
}

func (s *SQLStore) update(ctx context.Context, key string, destination any, reset func(bool) error, change func() error) error {
	if err := s.cfg.validateKey(key); err != nil {
		return err
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	keyColumn, valueColumn := "key", "value"
	if s.dialect == SQLMySQL {
		keyColumn, valueColumn = "`key`", "`value`"
	}
	selectValue := "SELECT " + valueColumn + " FROM " + s.quoted + " WHERE " + keyColumn + " = " + s.placeholder(1)
	const attempts = 8
	for attempt := 0; attempt < attempts; attempt++ {
		var previous []byte
		err := s.db.QueryRowContext(ctx, selectValue, key).Scan(&previous)
		existed := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return sqlFailure("read for update", err)
		}
		if err := reset(existed); err != nil {
			return err
		}
		if existed {
			if int64(len(previous)) > s.cfg.limits.MaxValueBytes+1024 {
				return ErrTooLarge
			}
			var current blob
			if err := json.Unmarshal(previous, &current); err != nil {
				return fmt.Errorf("stardb: decode SQL record: %w", err)
			}
			if err := s.cfg.decode(key, current, destination); err != nil {
				return err
			}
		}
		if err := change(); err != nil {
			return err
		}
		next, err := s.cfg.encode(key, destination)
		if err != nil {
			return err
		}
		packed, err := json.Marshal(next)
		if err != nil {
			return fmt.Errorf("stardb: encode SQL record: %w", err)
		}
		var result sql.Result
		if existed {
			if bytes.Equal(previous, packed) {
				return ctx.Err() // linearizes at the read; no write is needed
			}
			statement := "UPDATE " + s.quoted + " SET " + valueColumn + " = " + s.placeholder(1) + " WHERE " + keyColumn + " = " + s.placeholder(2) + " AND " + valueColumn + " = " + s.placeholder(3)
			result, err = s.db.ExecContext(ctx, statement, packed, key, previous)
		} else {
			var statement string
			switch s.dialect {
			case SQLPostgres:
				statement = "INSERT INTO " + s.quoted + " (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING"
			case SQLMySQL:
				// Do not use INSERT IGNORE: it can suppress truncation and other
				// data errors. A concurrent insert remains a driver error.
				statement = "INSERT INTO " + s.quoted + " (`key`, `value`) VALUES (?, ?)"
			default:
				statement = "INSERT INTO " + s.quoted + " (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING"
			}
			result, err = s.db.ExecContext(ctx, statement, key, packed)
		}
		if err != nil {
			return sqlFailure("atomic update", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return sqlFailure("read update result", err)
		}
		if affected == 0 {
			continue
		}
		s.cfg.log(ctx, slog.LevelDebug, "stardb update", "sql", key)
		return nil
	}
	return ErrConflict
}

func (s *SQLStore) Delete(ctx context.Context, key string) error {
	if err := s.cfg.validateKey(key); err != nil {
		return err
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.mu.RUnlock()
	keyColumn := "key"
	if s.dialect == SQLMySQL {
		keyColumn = "`key`"
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM "+s.quoted+" WHERE "+keyColumn+" = "+s.placeholder(1), key)
	if err != nil {
		return sqlFailure("delete", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return sqlFailure("read delete result", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	s.cfg.log(ctx, slog.LevelDebug, "stardb delete", "sql", key)
	return nil
}

func (s *SQLStore) Keys(ctx context.Context) ([]string, error) {
	if err := s.lock(ctx); err != nil {
		return nil, err
	}
	defer s.mu.RUnlock()
	keyColumn := "key"
	if s.dialect == SQLMySQL {
		keyColumn = "`key`"
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+keyColumn+" FROM "+s.quoted)
	if err != nil {
		return nil, sqlFailure("list keys", err)
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, sqlFailure("scan key", err)
		}
		if err := s.cfg.validateKey(key); err != nil {
			return nil, fmt.Errorf("stardb: invalid stored SQL key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, sqlFailure("list keys", err)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *SQLStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	zero(s.cfg.key)
	s.cfg.key = nil
	if s.cfg.ownSQL {
		if err := s.db.Close(); err != nil {
			return sqlFailure("close", err)
		}
	}
	return nil
}

func (s *SQLStore) lock(ctx context.Context) error {
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

func (s *SQLStore) placeholder(position int) string {
	if s.dialect == SQLPostgres {
		return fmt.Sprintf("$%d", position)
	}
	return "?"
}

func (d SQLDialect) String() string {
	switch d {
	case SQLPostgres:
		return "postgres"
	case SQLMySQL:
		return "mysql"
	default:
		return "sqlite"
	}
}
