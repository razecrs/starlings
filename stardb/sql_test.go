package stardb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
)

var (
	registerSQLTestDriver sync.Once
	testSQLDriver         = &memoryDriver{records: make(map[string][]byte)}
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	registerSQLTestDriver.Do(func() { sql.Register("stardb_memory", testSQLDriver) })
	testSQLDriver.mu.Lock()
	testSQLDriver.records = make(map[string][]byte)
	testSQLDriver.queries = nil
	testSQLDriver.mu.Unlock()
	db, err := sql.Open("stardb_memory", "")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSQLStoreEndToEndAndParameters(t *testing.T) {
	for _, dialect := range []SQLDialect{SQLSQLite, SQLPostgres, SQLMySQL} {
		t.Run(dialect.String(), func(t *testing.T) {
			db := openTestDB(t)
			defer db.Close()
			store, err := OpenSQL(db, "star_data", dialect, WithEncryption(make([]byte, 32)))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.EnsureSchema(context.Background()); err != nil {
				t.Fatal(err)
			}
			injectionKey := `x'; DROP TABLE star_data;--`
			want := testValue{Name: "Nebula", Count: 9}
			if err := store.Put(context.Background(), injectionKey, want); err != nil {
				t.Fatal(err)
			}
			var got testValue
			if err := store.Get(context.Background(), injectionKey, &got); err != nil || got.Name != want.Name {
				t.Fatalf("got=%#v err=%v", got, err)
			}
			keys, err := store.Keys(context.Background())
			if err != nil || len(keys) != 1 || keys[0] != injectionKey {
				t.Fatalf("keys=%v err=%v", keys, err)
			}
			if err := store.Delete(context.Background(), injectionKey); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(store.Get(context.Background(), injectionKey, &got), ErrNotFound) {
				t.Fatal("missing SQL key did not return ErrNotFound")
			}
			testSQLDriver.mu.Lock()
			queries := strings.Join(testSQLDriver.queries, "\n")
			testSQLDriver.mu.Unlock()
			if strings.Contains(queries, "DROP TABLE") {
				t.Fatalf("key was interpolated into SQL: %s", queries)
			}
		})
	}
}

func TestSQLStoreRejectsIdentifierInjection(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	if _, err := OpenSQL(db, "records; DROP TABLE records", SQLPostgres); err == nil {
		t.Fatal("unsafe SQL identifier accepted")
	}
}

func TestSQLErrorRedactsDriverText(t *testing.T) {
	cause := errors.New("constraint leaked private-user-value")
	err := sqlFailure("put", cause)
	if strings.Contains(err.Error(), "private-user-value") {
		t.Fatalf("driver text leaked through SQL error: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatal("SQL error no longer unwraps to its cause")
	}
}

func TestSQLStoreOwnedConnection(t *testing.T) {
	db := openTestDB(t)
	store, err := OpenSQL(db, "records", SQLSQLite, WithOwnedSQLConnection())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err == nil {
		t.Fatal("owned database remained open")
	}
}

type memoryDriver struct {
	mu      sync.Mutex
	records map[string][]byte
	queries []string
}

func (d *memoryDriver) Open(string) (driver.Conn, error) { return &memoryConn{driver: d}, nil }

type memoryConn struct{ driver *memoryDriver }

func (c *memoryConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not implemented")
}
func (c *memoryConn) Close() error { return nil }
func (c *memoryConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions not implemented")
}

func (c *memoryConn) ExecContext(_ context.Context, query string, arguments []driver.NamedValue) (driver.Result, error) {
	c.driver.mu.Lock()
	defer c.driver.mu.Unlock()
	c.driver.queries = append(c.driver.queries, query)
	upper := strings.ToUpper(strings.TrimSpace(query))
	switch {
	case strings.HasPrefix(upper, "CREATE TABLE"):
		return memoryResult(0), nil
	case strings.HasPrefix(upper, "INSERT INTO"):
		key, ok := arguments[0].Value.(string)
		if !ok {
			return nil, fmt.Errorf("key is not string")
		}
		value, ok := arguments[1].Value.([]byte)
		if !ok {
			return nil, fmt.Errorf("value is not bytes")
		}
		c.driver.records[key] = append([]byte(nil), value...)
		return memoryResult(1), nil
	case strings.HasPrefix(upper, "DELETE FROM"):
		key := arguments[0].Value.(string)
		if _, ok := c.driver.records[key]; !ok {
			return memoryResult(0), nil
		}
		delete(c.driver.records, key)
		return memoryResult(1), nil
	default:
		return nil, fmt.Errorf("unexpected exec: %s", query)
	}
}

func (c *memoryConn) QueryContext(_ context.Context, query string, arguments []driver.NamedValue) (driver.Rows, error) {
	c.driver.mu.Lock()
	defer c.driver.mu.Unlock()
	c.driver.queries = append(c.driver.queries, query)
	upper := strings.ToUpper(strings.TrimSpace(query))
	if strings.Contains(upper, " WHERE ") {
		key := arguments[0].Value.(string)
		value, ok := c.driver.records[key]
		if !ok {
			return &memoryRows{columns: []string{"value"}}, nil
		}
		return &memoryRows{columns: []string{"value"}, values: [][]driver.Value{{append([]byte(nil), value...)}}}, nil
	}
	keys := make([]string, 0, len(c.driver.records))
	for key := range c.driver.records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([][]driver.Value, len(keys))
	for i, key := range keys {
		values[i] = []driver.Value{key}
	}
	return &memoryRows{columns: []string{"key"}, values: values}, nil
}

var _ driver.ExecerContext = (*memoryConn)(nil)
var _ driver.QueryerContext = (*memoryConn)(nil)

type memoryResult int64

func (r memoryResult) LastInsertId() (int64, error) { return 0, nil }
func (r memoryResult) RowsAffected() (int64, error) { return int64(r), nil }

type memoryRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *memoryRows) Columns() []string { return r.columns }
func (r *memoryRows) Close() error      { return nil }
func (r *memoryRows) Next(destination []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(destination, r.values[r.index])
	r.index++
	return nil
}
